package checkpointcollector

import (
	"bytes"
	"context"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"k8s.io/klog"
	"k8s.io/klog/v2"
	"log"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	checkpoint "github.com/hth0919/checkpointproto"
	corev1 "ketiCRD/apis/keti/v1"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

const (
	port = ":20160"
)
type server struct {}

var Period int64

type containerList struct{
	containerID []string
	containerName []string
}
var cnumber int
var podList map[string]*containerList
var podchecker map[string]bool

func (s *server) SetCheckpointPeriod(ctx context.Context, in *checkpoint.InputValue) (*checkpoint.ReturnValue, error) {
	if *in.Period > 0 {
		Period = *in.Period
	}
	e := ""
	fmt.Println("set period from ",Period,"to", *in.Period)
	return &checkpoint.ReturnValue{
		Period:               in.Period,
		Err:                  &e,
	}, nil
}

func (s *server) CheckpointCreate(ctx context.Context, in *checkpoint.CreateCheckpoint) (*checkpoint.PodReturnValue, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}
	var podname string
	var id []string
	var name []string
	podname = *in.PodName
	explorer()
	fmt.Println(podList)
	fmt.Println(podname)
	if podchecker[podname] {
		id = podList[podname].containerID
		name = podList[podname].containerName
	}
	fmt.Println("create checkpoint ",*in.PodName)
	for i:=0;i<len(id);i++ {
		err := cli.CheckpointCreate(context.Background(), id[i], types.CheckpointCreateOptions{
			CheckpointID:  "cp"+string(0-cnumber),
			Exit:          false,
		})
		if err != nil {
			klog.Errorln(err)
		}
		containerinfo, err := cli.ContainerInspect(context.Background(), id[i])
		if err != nil {
			klog.Errorln(err)
		}
		hostnamepath := containerinfo.HostnamePath
		checkpointdir := strings.Replace(hostnamepath,"hostname", "checkpoints/*",1)
		nfspath := "/nfs/" + podname + "/" + name[i] + "/"
		cmd := exec.Command("cp", "-r", checkpointdir, nfspath)
		log.Printf("Running cp -r")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
		}
	}
	e := ""
	return &checkpoint.PodReturnValue{
		CheckpointName:      in.PodName,
		Err:                 &e,
	}, nil
}
func (s *server) StoreYaml(ctx context.Context, in *checkpoint.StoreValue) (*checkpoint.PodReturnValue, error) {
	path := filepath.Join("/", "migpod")
	pod := &corev1.Pod{}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			panic(err)
		}
	}
	fmt.Println("inputvalue : ",string(in.Yaml))
	err := yaml.Unmarshal(in.Yaml, pod)
	if err != nil {
		panic(err)
	}
	filename := path +"/"+ pod.Name + ".yaml"
	err = ioutil.WriteFile(filename, in.Yaml, 0)
	if err != nil {
		panic(err)
	}
	e := ""
	return &checkpoint.PodReturnValue{
		CheckpointName:       &pod.Name,
		Err:                  &e,
	}, nil
}


func createCheckpoint(){

	for {
		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			panic(err)
		}
		for k := range podList {
			fmt.Println("I'm in createCheckpoint!! :: ", k)
			container := podList[k]
			for i := 0; i < len(container.containerID); i++ {
				err := cli.CheckpointCreate(context.Background(), container.containerID[i], types.CheckpointCreateOptions{
					CheckpointID:  "cp"+string(cnumber),
					Exit:          false,
				})
				if err != nil {
					fmt.Println(err.Error())
				}
				containerinfo, err := cli.ContainerInspect(context.Background(), container.containerID[i])
				if err != nil {
					panic(err.Error())
				}
				hostnamepath := containerinfo.HostnamePath
				checkpointdir := strings.Replace(hostnamepath,"hostname", "checkpoints/.",1)
				nfspath := "/nfs/" + k + "/" + container.containerName[i] + "/"
				cmd := exec.Command("cp", "-r", checkpointdir, nfspath)
				log.Printf("Running cp -r")
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				err = cmd.Run()
				if err != nil {
					fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
					return
				}
			}
		}
		cnumber++
		time.Sleep(time.Second * time.Duration(Period))
	}
}

func explorer() {
	path := filepath.Join("/", "migpod")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			panic(err)
		}
	}
	files, err := ioutil.ReadDir(path)
	if err != nil {
		panic(err)
	}
	for _, f := range files {
		pod := &corev1.Pod{}
		containerlist := &containerList{
			containerID:   []string{},
			containerName: []string{},
		}
		fmt.Println("Filename : ", f.Name())
		bytes, err := ioutil.ReadFile(path + "/" + f.Name())
		if err != nil {
			panic(err)
		}
		err = yaml.Unmarshal(bytes, pod)
		if err != nil {
			panic(err)
		}
		podList[pod.Name] = containerlist
		podchecker[pod.Name] = true
	}
}


func podLister() {
	for {
		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			panic(err)
		}

		containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{})
		if err != nil {
			panic(err)
		}
		explorer()

		for _, container := range containers{
			pod := container.Labels["io.kubernetes.pod.name"]
			if podchecker[pod] {
				podList[pod].containerID = append(podList[pod].containerID, container.ID)
				podList[pod].containerName = append(podList[pod].containerName, container.Labels["io.kubernetes.container.name"])
			}
		}
		time.Sleep(time.Second * 5)
	}
}


func NewCheckpointCollector() (net.Listener, *grpc.Server) {
	runtime.GOMAXPROCS(4)
	cnumber = 1
	podList = make(map[string]*containerList)
	podchecker = make(map[string]bool)
	Period = 20
	go podLister()
	go createCheckpoint()

	l, err := net.Listen("tcp", port)
	if err != nil {
		klog.Errorln("failed to listen : ", err)
	}

	s := grpc.NewServer()
	checkpoint.RegisterCheckpointPeriodServer(s, &server{})
	klog.Infoln("grpc init finish")
	return l,s

}