package bootkube

import (
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/spf13/pflag"
	apiapp "k8s.io/kubernetes/cmd/kube-apiserver/app"
	apiserver "k8s.io/kubernetes/cmd/kube-apiserver/app/options"
	cmapp "k8s.io/kubernetes/cmd/kube-controller-manager/app"
	controller "k8s.io/kubernetes/cmd/kube-controller-manager/app/options"
	schedapp "k8s.io/kubernetes/plugin/cmd/kube-scheduler/app"
	scheduler "k8s.io/kubernetes/plugin/cmd/kube-scheduler/app/options"

	"github.com/kubernetes-incubator/bootkube/pkg/asset"
)

const (
	assetTimeout    = 10 * time.Minute
	insecureAPIAddr = "http://127.0.0.1:8080"
)

var requiredPods = []string{
	"kube-api-checkpoint",
	"kube-apiserver",
	"kube-scheduler",
	"kube-controller-manager",
}

type Config struct {
	AssetDir   string
	EtcdServer *url.URL
	EtcdPrefix string
}

type bootkube struct {
	assetDir   string
	apiServer  *apiserver.APIServer
	controller *controller.CMServer
	scheduler  *scheduler.SchedulerServer
}

func NewBootkube(config Config) (*bootkube, error) {
	apiServer := apiserver.NewAPIServer()
	fs := pflag.NewFlagSet("apiserver", pflag.ExitOnError)
	apiServer.AddFlags(fs)
	args := []string{
		"--bind-address=0.0.0.0",
		"--secure-port=443",
		"--insecure-port=8080",
		"--allow-privileged=true",
		"--tls-private-key-file=" + filepath.Join(config.AssetDir, asset.AssetPathAPIServerKey),
		"--tls-cert-file=" + filepath.Join(config.AssetDir, asset.AssetPathAPIServerCert),
		"--client-ca-file=" + filepath.Join(config.AssetDir, asset.AssetPathCACert),
		"--etcd-servers=" + config.EtcdServer.String(),
		"--service-cluster-ip-range=10.3.0.0/24",
		"--service-account-key-file=" + filepath.Join(config.AssetDir, asset.AssetPathServiceAccountPubKey),
		"--admission-control=NamespaceLifecycle,ServiceAccount",
		"--runtime-config=api/all=true",
	}
	if config.EtcdPrefix != "" {
		args = append(args, "--etcd-prefix="+config.EtcdPrefix)
	}
	fs.Parse(args)
	cmServer := controller.NewCMServer()
	fs = pflag.NewFlagSet("controllermanager", pflag.ExitOnError)
	cmServer.AddFlags(fs)
	fs.Parse([]string{
		"--master=" + insecureAPIAddr,
		"--service-account-private-key-file=" + filepath.Join(config.AssetDir, asset.AssetPathServiceAccountPrivKey),
		"--root-ca-file=" + filepath.Join(config.AssetDir, asset.AssetPathCACert),
		"--leader-elect=true",
	})

	schedServer := scheduler.NewSchedulerServer()
	fs = pflag.NewFlagSet("scheduler", pflag.ExitOnError)
	schedServer.AddFlags(fs)
	fs.Parse([]string{
		"--master=" + insecureAPIAddr,
		"--leader-elect=true",
	})

	return &bootkube{
		apiServer:  apiServer,
		controller: cmServer,
		scheduler:  schedServer,
		assetDir:   config.AssetDir,
	}, nil
}

func (b *bootkube) Run() error {
	UserOutput("Running temporary bootstrap control plane...\n")

	errch := make(chan error)
	go func() { errch <- apiapp.Run(b.apiServer) }()
	go func() { errch <- cmapp.Run(b.controller) }()
	go func() { errch <- schedapp.Run(b.scheduler) }()
	go func() {
		if err := CreateAssets(filepath.Join(b.assetDir, asset.AssetPathManifests), assetTimeout); err != nil {
			errch <- err
		}
	}()
	go func() { errch <- WaitUntilPodsRunning(requiredPods, assetTimeout) }()

	// If any of the bootkube services exit, it means it is unrecoverable and we should exit.
	err := <-errch
	if err != nil {
		UserOutput("Error: %v\n", err)
	}
	return err
}

// All bootkube printing to stdout should go through this fmt.Printf wrapper.
// The stdout of bootkube should convey information useful to a human sitting
// at a terminal watching their cluster bootstrap itself. Otherwise the message
// should go to stderr.
func UserOutput(format string, a ...interface{}) {
	fmt.Printf(format, a...)
}
