package v1
import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type PodV1Interface interface {
	RESTClient() rest.Interface
	Pods(namespace string) PodInterface
}

type PodV1Client struct {
	restClient rest.Interface
	pod        *PodClient
}

func NewForConfig(c *rest.Config) (*PodV1Client, error) {
	config := *c
	config.ContentConfig.GroupVersion = &schema.GroupVersion{Group: "keti.migration", Version: "v1"}
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &PodV1Client{restClient: client}, nil
}

func (c *PodV1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}

func (c *PodV1Client) Pods(namespace string) PodInterface {
	return newMigrationPodClient(c,namespace)
}