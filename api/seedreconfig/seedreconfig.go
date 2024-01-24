package seedreconfig

type PEM string

const (
	SeedReconfigurationVersion = 1
)

// SeedReconfiguration contains all the information that is required to
// transform a machine started from a SNO seed image (which contains dummy seed
// configuration) into a SNO cluster with a desired configuration. During an
// IBU, this information is taken from the cluster that is being upgraded (the
// original SNO's LCA writes a file to the seed stateroot to communicate this
// information). During an IBI, this information is placed in the configuration
// ISO created by the image-based-install-operator.
//
// WARNING: Changes to this struct and its sub-structs should not be made
// lightly, as it is also used by the image-based-install-operator. Any changes
// made here will also need to be handled in the image-based-install-operator
// or be backwards compatible. If you've made a breaking change, you will need
// to increment the SeedReconfigVersion constant to avoid silent breakage and
// allow for backwards compatibility code.
type SeedReconfiguration struct {
	// The version of the SeedReconfiguration struct format. This is used to detect
	// breaking changes to the struct.
	APIVersion int `json:"api_version"`

	// The desired base domain for the cluster. Equivalent to install-config.yaml's baseDomain.
	// This will replace the base domain of the seed cluster.
	BaseDomain string `json:"base_domain,omitempty"`

	// The desired cluster name for the cluster. Equivalent to install-config.yaml's clusterName.
	// This will replace the cluster name of the seed cluster.
	ClusterName string `json:"cluster_name,omitempty"`

	// The desired cluster-ID. During an IBU, this is the ID of the original
	// SNO. During an IBI, this can either be empty, in which case a new
	// cluster-ID will be generated by LCA, or it can be set to the ID of the
	// new cluster, in case one had to be pre-generated for some reason.
	ClusterID string `json:"cluster_id,omitempty"`

	// The desired IP address of the SNO node.
	NodeIP string `json:"node_ip,omitempty"`

	// The container registry used to host the release image of the seed cluster.
	ReleaseRegistry string `json:"release_registry,omitempty"`

	// The desired hostname of the SNO node.
	Hostname string `json:"hostname,omitempty"`

	// KubeconfigCryptoRetention contains all the crypto material that is
	// required for recert to ensure existing kubeconfigs can be used to access
	// the cluster after recert.
	//
	// In the case of UBI, this material is taken from the cluster that is
	// being upgraded. In the case of IBI, this material is generated when the
	// cluster's kubeconfig is being prepared in advance.
	KubeconfigCryptoRetention KubeConfigCryptoRetention

	// SSHKey is the public Secure Shell (SSH) key to provide access to
	// instances. Equivalent to install-config.yaml's sshKey. This will replace
	// the SSH keys of the seed cluster.
	SSHKey string `json:"ssh_key,omitempty"`

	// The pull secret that obtained from the Pull Secret page on the Red Hat OpenShift Cluster Manager site.
	PullSecret string `json:"pull_secret,omitempty"`
}

type KubeConfigCryptoRetention struct {
	KubeAPICrypto  KubeAPICrypto
	IngresssCrypto IngresssCrypto
}

type KubeAPICrypto struct {
	ServingCrypto    ServingCrypto
	ClientAuthCrypto ClientAuthCrypto
}

type ServingCrypto struct {
	LocalhostSignerPrivateKey      PEM `json:"localhost_signer_private_key,omitempty"`
	ServiceNetworkSignerPrivateKey PEM `json:"service_network_signer_private_key,omitempty"`
	LoadbalancerSignerPrivateKey   PEM `json:"loadbalancer_external_signer_private_key,omitempty"`
}

type ClientAuthCrypto struct {
	AdminCACertificate PEM `json:"admin_ca_certificate,omitempty"`
}

type IngresssCrypto struct {
	IngressCA PEM `json:"ingress_ca,omitempty"`
}
