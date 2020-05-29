package kubelet

import (
	"context"
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/hth0919/ketiCRD/pkg/update"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/apis/podresources"
	"k8s.io/kubernetes/pkg/kubelet/config"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/events"
	"k8s.io/kubernetes/pkg/kubelet/eviction"
	"k8s.io/kubernetes/pkg/kubelet/pleg"
	"k8s.io/kubernetes/pkg/kubelet/server/streaming"
	"k8s.io/kubernetes/pkg/kubelet/status"
	"k8s.io/kubernetes/pkg/kubelet/util"
	"k8s.io/kubernetes/pkg/kubelet/util/format"
	"k8s.io/kubernetes/pkg/kubelet/util/sliceutils"
	"ketiCRD/client"
	"ketiCRD/checkpointcollector"
	kubetypes "github.com/hth0919/ketiCRD/pkg/types"
	podmanager "github.com/hth0919/ketiCRD/pkg/pod"
	dockerclient "github.com/docker/docker/client"
	"math"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	v1 "ketiCRD/apis/keti/v1"
	corev1 "k8s.io/api/core/v1"
)
type Options struct {
	Kubeconfig *string
	Nodename *string
}
func Run(opt Options){
	kl := &KETIkubelet{}
	kl.init(opt)
	kl.run()
}


type SyncHandler interface {
	HandlePodAdditions(pods []*corev1.Pod)
	HandlePodUpdates(pods []*corev1.Pod)
	HandlePodRemoves(pods []*corev1.Pod)
	HandlePodReconcile(pods []*corev1.Pod)
	HandlePodSyncs(pods []*corev1.Pod)
	HandlePodCleanups() error
}



const (
	port = ":20150"
)
type server struct {
	currentpod *v1.Pod
	pods []*v1.Pod
	updatechan chan kubetypes.PodUpdate
}

func(s *server) SendPod(ctx context.Context, in *PodInfo) (*Empty, error) {
	s.currentpod = yamlToStruct(in.Yaml)

	s.pods = append(s.pods, s.currentpod)
	pu := kubetypes.PodUpdate{
		Pods:   s.pods,
		Op:     kubetypes.PodOperation(in.Operation),
		Source: kubetypes.GRPCSource,
	}
	s.updatechan<-pu
	return &Empty{}, nil
}
func(s *server) init(pods []*v1.Pod, updatechan chan kubetypes.PodUpdate) {
	s.pods = pods
	s.updatechan = updatechan
}

func yamlToStruct(yamlfile []byte) *v1.Pod {
	out := &v1.Pod{}
	err := yaml.Unmarshal(yamlfile, out)
	if err != nil {
		klog.Errorln(err)
	}
	return out
}
func structToYaml(pod interface{}) []byte {
	out, err := yaml.Marshal(pod)
	if err != nil {
		klog.Errorln(err)
	}
	return out
}


type KETIkubelet struct {
	clientset *kubernetes.Clientset
	migrationclientset *client.MigrationV1Client
	podUpdate kubetypes.PodUpdate
	grpcserver server
	handler SyncHandler
	nodename string
	updatechan chan kubetypes.PodUpdate
	podManager podmanager.Manager
	clock clock.Clock
	dockerManager *dockerclient.Client
	checkpointCollector *grpc.Server
	podWorker podmanager.PodWorkers
}


func configKubelet(opt Options) (*kubernetes.Clientset, *client.MigrationV1Client){
	// use the current context in kubeconfig
	cfg, err := clientcmd.BuildConfigFromFlags("", *opt.Kubeconfig)
	if err != nil {
		klog.Errorln(err)
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Errorln(err)
	}

	migrationclientset, err := client.NewForConfig(cfg)
	if err != nil {
		klog.Errorln(err)
	}

	return clientset, migrationclientset
}
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}


func (kl *KETIkubelet)run() {

}
func (kl *KETIkubelet)init(opt Options) {
	var listener net.Listener
	var err error
	kl.updatechan = update.NewUpdateChannel()
	kl.clientset, kl.migrationclientset = configKubelet(opt)
	kl.nodename = *opt.Nodename
	kl.podUpdate = initPodUpdate(kl)
	kl.podManager = podmanager.NewBasicPodManager(nil,nil)
	kl.grpcserver.init(kl.podUpdate.Pods, kl.updatechan)
	kl.dockerManager, err = dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		klog.Errorln(err)
	}
	listener, kl.checkpointCollector = checkpointcollector.NewCheckpointCollector()
	go func() {
		if err = kl.checkpointCollector.Serve(listener); err!= nil{
			klog.Errorln("Cannot Serve : ", err)
		}
	}()

}

func (kl *KETIkubelet) syncLoop(updates <-chan kubetypes.PodUpdate, handler SyncHandler) {
	klog.Info("Starting kubelet main sync loop.")
	// The syncTicker wakes up kubelet to checks if there are any pod workers
	// that need to be sync'd. A one-second period is sufficient because the
	// sync interval is defaulted to 10s.
	syncTicker := time.NewTicker(time.Second)
	defer syncTicker.Stop()
	housekeepingTicker := time.NewTicker(housekeepingPeriod)
	defer housekeepingTicker.Stop()
	plegCh := kl.pleg.Watch()
	const (
		base   = 100 * time.Millisecond
		max    = 5 * time.Second
		factor = 2
	)
	duration := base
	// Responsible for checking limits in resolv.conf
	// The limits do not have anything to do with individual pods
	// Since this is called in syncLoop, we don't need to call it anywhere else
	if kl.dnsConfigurer != nil && kl.dnsConfigurer.ResolverConfig != "" {
		kl.dnsConfigurer.CheckLimitsForResolvConf()
	}

	for {
		if err := kl.runtimeState.runtimeErrors(); err != nil {
			klog.Errorf("skipping pod synchronization - %v", err)
			// exponential backoff
			time.Sleep(duration)
			duration = time.Duration(math.Min(float64(max), factor*float64(duration)))
			continue
		}
		// reset backoff if we have a success
		duration = base

		kl.syncLoopMonitor.Store(kl.clock.Now())
		if !kl.syncLoopIteration(updates, handler, syncTicker.C, housekeepingTicker.C, plegCh) {
			break
		}
		kl.syncLoopMonitor.Store(kl.clock.Now())
	}
}




