package asset

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"

	"github.com/kubernetes-incubator/bootkube/pkg/tlsutil"
)

const (
	AssetPathCAKey                   = "tls/ca.key"
	AssetPathCACert                  = "tls/ca.crt"
	AssetPathAPIServerKey            = "tls/apiserver.key"
	AssetPathAPIServerCert           = "tls/apiserver.crt"
	AssetPathServiceAccountPrivKey   = "tls/service-account.key"
	AssetPathServiceAccountPubKey    = "tls/service-account.pub"
	AssetPathKubeletKey              = "tls/kubelet.key"
	AssetPathKubeletCert             = "tls/kubelet.crt"
	AssetPathKubeConfig              = "auth/kubeconfig"
	AssetPathManifests               = "manifests"
	AssetPathKubelet                 = "manifests/kubelet.yaml"
	AssetPathProxy                   = "manifests/kube-proxy.yaml"
	AssetPathAPIServerSecret         = "manifests/kube-apiserver-secret.yaml"
	AssetPathAPIServer               = "manifests/kube-apiserver.yaml"
	AssetPathControllerManager       = "manifests/kube-controller-manager.yaml"
	AssetPathControllerManagerSecret = "manifests/kube-controller-manager-secret.yaml"
	AssetPathScheduler               = "manifests/kube-scheduler.yaml"
	AssetPathKubeDNSDeployment       = "manifests/kube-dns-deployment.yaml"
	AssetPathKubeDNSSvc              = "manifests/kube-dns-svc.yaml"
	AssetPathSystemNamespace         = "manifests/kube-system-ns.yaml"
)

// AssetConfig holds all configuration needed when generating
// the default set of assets.
type Config struct {
	EtcdServers     []*url.URL
	APIServers      []*url.URL
	CACert          *x509.Certificate
	CAPrivKey       *rsa.PrivateKey
	AltNames        *tlsutil.AltNames
	SelfHostKubelet bool
	CloudProvider   string
	EtcdPrefix      string
}

// NewDefaultAssets returns a list of default assets, optionally
// configured via a user provided AssetConfig. Default assets include
// TLS assets (certs, keys and secrets), and k8s component manifests.
func NewDefaultAssets(conf Config) (Assets, error) {
	as := newStaticAssets(conf.SelfHostKubelet)
	as = append(as, newDynamicAssets(conf)...)

	// TLS assets
	tlsAssets, err := newTLSAssets(conf.CACert, conf.CAPrivKey, *conf.AltNames)
	if err != nil {
		return Assets{}, err
	}
	as = append(as, tlsAssets...)

	// K8S kubeconfig
	kubeConfig, err := newKubeConfigAsset(as, conf)
	if err != nil {
		return Assets{}, err
	}
	as = append(as, kubeConfig)

	// K8S APIServer secret
	apiSecret, err := newAPIServerSecretAsset(as)
	if err != nil {
		return Assets{}, err
	}
	as = append(as, apiSecret)

	// K8S ControllerManager secret
	cmSecret, err := newControllerManagerSecretAsset(as)
	if err != nil {
		return Assets{}, err
	}
	as = append(as, cmSecret)

	return as, nil
}

type Asset struct {
	Name string
	Data []byte
}

type Assets []Asset

func (as Assets) Get(name string) (Asset, error) {
	for _, asset := range as {
		if asset.Name == name {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("asset %q does not exist", name)
}

func (as Assets) WriteFiles(path string) error {
	if err := os.Mkdir(path, 0755); err != nil {
		return err
	}
	for _, asset := range as {
		f := filepath.Join(path, asset.Name)
		if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
			return err
		}
		fmt.Printf("Writing asset: %s\n", f)
		if err := ioutil.WriteFile(f, asset.Data, 0600); err != nil {
			return err
		}
	}
	return nil
}
