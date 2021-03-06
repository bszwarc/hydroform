package operator

import "github.com/kyma-incubator/hydroform/types"

type Operator interface {
	Create(providerType types.ProviderType, configuration map[string]interface{}) (*types.ClusterInfo, error)
	Delete(state *types.InternalState, providerType types.ProviderType, configuration map[string]interface{}) error
}

func NewTerraform() Operator {
	return &Terraform{}
}

type OperatorType string

const (
	TerraformOperator OperatorType = "terraform"
)
