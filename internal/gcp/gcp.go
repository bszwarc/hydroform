package gcp

import (
	"context"
	"fmt"

	"cloud.google.com/go/container"
	"github.com/kyma-incubator/hydroform/internal/operator"
	"github.com/kyma-incubator/hydroform/types"
	"github.com/pkg/errors"
	"google.golang.org/api/option"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

var mandatoryConfigFields = []string{}

// gcpProvisioner implements Provisioner
type gcpProvisioner struct {
	provisionOperator operator.Operator
}

// Provision requests provisioning of a new Kubernetes cluster on GCP with the given configurations.
func (g *gcpProvisioner) Provision(cluster *types.Cluster, provider *types.Provider) (*types.Cluster, error) {
	if !g.validateProvider(provider) {
		return nil, errors.New("incomplete provider information")
	}

	config := g.loadConfigurations(cluster, provider)

	clusterInfo, err := g.provisionOperator.Create(provider.Type, config)
	if err != nil {
		return cluster, errors.Wrap(err, "unable to provision gcp cluster")
	}

	cluster.ClusterInfo = clusterInfo
	return cluster, nil
}

// Status returns the ClusterStatus for the requested cluster.
func (g *gcpProvisioner) Status(cluster *types.Cluster, provider *types.Provider) (*types.ClusterStatus, error) {
	containerClient, err := container.NewClient(context.Background(),
		provider.ProjectName,
		option.WithCredentialsFile(provider.CredentialsFilePath))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create GCP client")
	}
	cl, err := containerClient.Cluster(context.Background(), cluster.Location, cluster.Name)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get cluster info")
	}

	return &types.ClusterStatus{
		Phase: g.convertGCPStatus(cl.Status),
	}, nil
}

// Credentials returns the Kubeconfig file as a byte array for the requested cluster.
func (g *gcpProvisioner) Credentials(cluster *types.Cluster, provider *types.Provider) ([]byte, error) {
	userName := "cluster-user"
	config := api.NewConfig()

	config.Clusters[cluster.Name] = &api.Cluster{
		Server:                   fmt.Sprintf("https://%v", cluster.ClusterInfo.Endpoint),
		CertificateAuthorityData: cluster.ClusterInfo.CertificateAuthorityData,
	}

	config.Contexts[cluster.Name] = &api.Context{
		Cluster:  cluster.Name,
		AuthInfo: userName,
	}

	config.CurrentContext = cluster.Name

	config.AuthInfos[userName] = &api.AuthInfo{
		AuthProvider: &api.AuthProviderConfig{
			Name: "gcp",
		},
	}

	return clientcmd.Write(*config)
}

// Deprovision requests deprovisioning of an existing cluster on GCP with the given configurations.
func (g *gcpProvisioner) Deprovision(cluster *types.Cluster, provider *types.Provider) error {
	if !g.validateProvider(provider) {
		return errors.New("incomplete provider information")
	}

	config := g.loadConfigurations(cluster, provider)

	err := g.provisionOperator.Delete(cluster.ClusterInfo.InternalState, provider.Type, config)
	if err != nil {
		return errors.Wrap(err, "unable to deprovision gcp cluster")
	}

	return nil
}

// New creates a new instance of gcpProvisioner.
func New(operatorType operator.Type) *gcpProvisioner {
	var op operator.Operator

	switch operatorType {
	case operator.TerraformOperator:
		op = &operator.Terraform{}
	default:
		op = &operator.Unknown{}
	}

	return &gcpProvisioner{
		provisionOperator: op,
	}
}

func (g *gcpProvisioner) validateProvider(provider *types.Provider) bool {
	for _, field := range mandatoryConfigFields {
		if _, ok := provider.CustomConfigurations[field]; !ok {
			return false
		}
	}
	return true
}

func (g *gcpProvisioner) loadConfigurations(cluster *types.Cluster, provider *types.Provider) map[string]interface{} {
	config := map[string]interface{}{}
	config["cluster_name"] = cluster.Name
	config["node_count"] = cluster.NodeCount
	config["machine_type"] = cluster.MachineType
	config["disk_size"] = cluster.DiskSizeGB
	config["kubernetes_version"] = cluster.KubernetesVersion
	config["location"] = cluster.Location
	config["project"] = provider.ProjectName
	config["credentials_file_path"] = provider.CredentialsFilePath
	for k, v := range provider.CustomConfigurations {
		config[k] = v
	}
	return config
}

// Possible values for the GCP Cluster Status:
//   "STATUS_UNSPECIFIED" - not set.
//   "PROVISIONING" - indicates the cluster is being created.
//   "RUNNING" - indicates the cluster has been created and is fully usable.
//   "RECONCILING" - indicates that some work is actively being done on the cluster,
//                   such as upgrading the master or node software.
//   "STOPPING" - indicates the cluster is being deleted.
//   "ERROR" - indicates the cluster may be unusable.
//   "DEGRADED" - indicates the cluster requires user action to restore full functionality.
// More details can be found in the `statusMessage` field.
func (g *gcpProvisioner) convertGCPStatus(status container.Status) types.Phase {
	switch status {
	default:
		return types.Unknown
	case "PROVISIONING":
		return types.Provisioning
	case "RUNNING":
		return types.Provisioned
	case "RECONCILING":
		return types.Pending
	case "STOPPING":
		return types.Stopping
	case "ERROR":
		return types.Errored
	case "DEGRADED":
		return types.Errored
	}
}
