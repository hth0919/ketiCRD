module ketiCRD/checkpointcollector

go 1.13

require (
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.13.1
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/hth0919/checkpointproto v0.0.3
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/stretchr/testify v1.5.1 // indirect
	google.golang.org/grpc v1.29.1
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.0.0
	ketiCRD/apis v0.0.0-00010101000000-000000000000
)

replace ketiCRD/apis => ../apis
