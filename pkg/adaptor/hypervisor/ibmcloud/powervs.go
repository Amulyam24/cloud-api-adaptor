// (C) Copyright IBM Corp. 2022.
// SPDX-License-Identifier: Apache-2.0
//go:build ibmcloud

package ibmcloud

import (
	"context"

	"github.com/IBM-Cloud/power-go-client/clients/instance"
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM-Cloud/power-go-client/power/models"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/platform-services-go-sdk/iamidentityv1"
)

type Service struct {
	session        *ibmpisession.IBMPISession
	instanceClient *instance.IBMPIInstanceClient
}

type PowerVS interface {
	CreateInstance(body *models.PVMInstanceCreate) (*models.PVMInstanceList, error)
	DeleteInstance(instanceId string) error
	Get(instanceId string) (*models.PVMInstance, error)
}

func (s *Service) CreateInstance(body *models.PVMInstanceCreate) (*models.PVMInstanceList, error) {
	return s.instanceClient.Create(body)
}

func (s *Service) DeleteInstance(instanceId string) error {
	return s.instanceClient.Delete(instanceId)
}

func (s *Service) Get(instanceId string) (*models.PVMInstance, error) {
	return s.instanceClient.Get(instanceId)
}

func NewPowervsService(apikey, serviceinstanceID, zone string) (PowerVS, error) {
	options := &ibmpisession.IBMPIOptions{}
	options.Authenticator = &core.IamAuthenticator{
		ApiKey: apikey,
	}
	ic, err := newIdentityClient(options.Authenticator)
	if err != nil {
		return nil, err
	}

	account, err := getAccount(apikey, ic)
	if err != nil {
		return nil, err
	}
	options.UserAccount = *account
	options.Zone = zone

	session, err := ibmpisession.NewIBMPISession(options)
	if err != nil {
		return nil, err
	}

	return &Service{
		session:        session,
		instanceClient: instance.NewIBMPIInstanceClient(context.Background(), session, serviceinstanceID),
	}, nil
}

func newIdentityClient(auth core.Authenticator) (*iamidentityv1.IamIdentityV1, error) {
	identityv1Options := &iamidentityv1.IamIdentityV1Options{
		Authenticator: auth,
	}
	identityClient, err := iamidentityv1.NewIamIdentityV1(identityv1Options)
	if err != nil {
		return nil, err
	}
	return identityClient, nil
}

func getAccount(key string, identityClient *iamidentityv1.IamIdentityV1) (*string, error) {
	apikeyDetailsOptions := &iamidentityv1.GetAPIKeysDetailsOptions{
		IamAPIKey: &key,
	}

	apiKeyDetails, _, err := identityClient.GetAPIKeysDetails(apikeyDetailsOptions)
	if err != nil {
		return nil, err
	}
	return apiKeyDetails.AccountID, nil
}
