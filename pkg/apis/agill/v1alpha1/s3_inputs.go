package v1alpha1

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"
)

type userPolicy struct {
	Version    string                `json:"Version"`
	ID         string                `json:"Id"`
	Statements []userPolicyStatement `json:"Statement"`
}

type userPolicyStatement struct {
	SID      string   `json:"Sid"`
	Effect   string   `json:"Effect"`
	Action   []string `json:"Action"`
	Resource []string `json:"Resource"`
}

func DesiredRestrictedPolicyDocForBucket(policyName string, bucketName string) (string, error) {
	userPolicy := userPolicy{
		Version: "2012-10-17",
		ID:      policyName,
		Statements: []userPolicyStatement{
			{
				SID:      "1",
				Effect:   "Allow",
				Action:   []string{"s3:*"},
				Resource: []string{fmt.Sprintf("arn:aws:s3:::%v", bucketName)},
			},
			{
				SID:      "2",
				Effect:   "Allow",
				Action:   []string{"s3:*"},
				Resource: []string{fmt.Sprintf("arn:aws:s3:::%v/*", bucketName)},
			},
		},
	}

	policy, err := json.Marshal(userPolicy)
	if err != nil {
		return "", err
	}

	return string(policy), nil
}

func (s S3) CreateBucketIn() *s3.CreateBucketInput {
	s3Input := &s3.CreateBucketInput{
		Bucket: aws.String(s.Spec.BucketName),
	}
	if s.Spec.Region != "us-east-1" {
		s3Input.CreateBucketConfiguration = s.SetBucketLocation()
	}

	if s.Spec.EnableObjectLock {
		s3Input.ObjectLockEnabledForBucket = aws.Bool(s.Spec.EnableObjectLock)
	}

	return s3Input
}

func (s S3) PutBucketAclIn() *s3.PutBucketAclInput {
	return &s3.PutBucketAclInput{
		ACL:    aws.String(s.Spec.BucketACL),
		Bucket: aws.String(s.Spec.BucketName),
	}
}

func (s S3) PutBucketVersioningIn() *s3.PutBucketVersioningInput {
	var enableStatus = s3.BucketVersioningStatusSuspended
	if s.Spec.EnableVersioning {
		enableStatus = s3.BucketAccelerateStatusEnabled
	}
	return &s3.PutBucketVersioningInput{
		Bucket: aws.String(s.Spec.BucketName),
		MFA:    nil,
		VersioningConfiguration: &s3.VersioningConfiguration{
			Status: aws.String(enableStatus),
		},
	}
}

func (s S3) PutBucketAccelIn() *s3.PutBucketAccelerateConfigurationInput {
	var status = s3.BucketAccelerateStatusSuspended
	if s.Spec.EnableTransferAcceleration {
		status = s3.BucketAccelerateStatusEnabled
	}
	return &s3.PutBucketAccelerateConfigurationInput{
		AccelerateConfiguration: &s3.AccelerateConfiguration{Status: aws.String(status)},
		Bucket:                  aws.String(s.Spec.BucketName),
	}
}

func (s S3) DeleteBucketIn() *s3.DeleteBucketInput {
	return &s3.DeleteBucketInput{Bucket: aws.String(s.Spec.BucketName)}
}

func (s S3) PutBucketPolicyIn() *s3.PutBucketPolicyInput {
	return &s3.PutBucketPolicyInput{
		Bucket: aws.String(s.Spec.BucketName),
		Policy: aws.String(s.Spec.BucketPolicy),
	}
}

func (s S3) SetBucketLocation() *s3.CreateBucketConfiguration {
	if s.Spec.Region != "" {
		return &s3.CreateBucketConfiguration{LocationConstraint: aws.String(s.Spec.Region)}
	}
	return nil
}

func (s S3) CreateIAMUserIn() *iam.CreateUserInput {
	iamUserIn := &iam.CreateUserInput{
		UserName: aws.String(s.Spec.IAMUserSpec.Username),
	}
	return iamUserIn
}

func (s S3) GetPolicyName() string {
	return fmt.Sprintf("%v-%v-s3-restricted", s.Spec.IAMUserSpec.Username, s.Spec.BucketName)
}

func (s S3) GetUsername() string {
	return s.Spec.IAMUserSpec.Username
}

func (s S3) GetIAMK8SSecretName() string {
	return fmt.Sprintf("%v-iam-secret", s.GetName())
}

func (s S3) GetRestrictedInlinePolicyInput() (*iam.PutUserPolicyInput, error) {
	policyDoc, err := DesiredRestrictedPolicyDocForBucket(s.GetPolicyName(), s.Spec.BucketName)
	if err != nil {
		return nil, err
	}
	return &iam.PutUserPolicyInput{
		PolicyDocument: aws.String(policyDoc),
		PolicyName:     aws.String(s.GetPolicyName()),
		UserName:       aws.String(s.Spec.IAMUserSpec.Username),
	}, nil
}
