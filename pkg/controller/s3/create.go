package s3

import (
	"context"
	"github.com/agill17/s3-operator/pkg/apis/agill/v1alpha1"
	customErrors "github.com/agill17/s3-operator/pkg/controller/errors"
	"github.com/agill17/s3-operator/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/davecgh/go-spew/spew"
	v1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r ReconcileS3) createBucket(cr *v1alpha1.S3) error {
	exists, errGettingBucket := utils.BucketExists(cr.Spec.BucketName, r.s3Client)
	if errGettingBucket != nil {
		r.recorder.Eventf(cr, v1.EventTypeWarning, "FAILED", "Failed to get bucket from Cloud: %v", errGettingBucket)
		return errGettingBucket
	}

	if !exists {
		r.recorder.Eventf(cr, v1.EventTypeNormal, "CREATING", "Bucket does not exist, creating now...")
		out, err := r.s3Client.CreateBucket(cr.CreateBucketIn())
		if err != nil {
			r.recorder.Eventf(cr, v1.EventTypeWarning, "FAILED", "Failed to create bucket: %v", err)
			return err
		}
		spew.Dump(out)
		r.recorder.Eventf(cr, v1.EventTypeNormal, "CREATED", "S3 Bucket created successfully")
	}

	if _, errPuttingBucketAcl := r.s3Client.PutBucketAcl(cr.PutBucketAclIn()); errPuttingBucketAcl != nil {
		return errPuttingBucketAcl
	}

	if _, errPuttingBucketVersionong := r.s3Client.PutBucketVersioning(cr.PutBucketVersioningIn()); errPuttingBucketVersionong != nil {
		return errPuttingBucketVersionong
	}

	if _, errPuttingBucketAcceleration := r.s3Client.PutBucketAccelerateConfiguration(cr.PutBucketAccelIn()); errPuttingBucketAcceleration != nil {
		return errPuttingBucketAcceleration
	}

	return PutBucketPolicy(cr, r.s3Client)
}

func PutBucketPolicy(cr *v1alpha1.S3, s3Client s3iface.S3API) error {

	if cr.Spec.BucketPolicy == "" {
		_, errDeletingBucketPolicy := s3Client.DeleteBucketPolicy(&s3.DeleteBucketPolicyInput{Bucket: aws.String(cr.Spec.BucketName)})
		return errDeletingBucketPolicy
	}

	input := cr.PutBucketPolicyIn()
	if err := input.Validate(); err != nil {
		return err
	}
	if _, err := s3Client.PutBucketPolicy(input); err != nil {
		return err
	}
	return nil

}

// if secret is not found in namespace, create new access keys ( delete the rest of the access keys if any )
// if secret is found, and access key does not match IAM access key ( delete the secret and delete all access keys on IAM ) and create fresh access keys
func handleAccessKeys(cr *v1alpha1.S3, iamClient iamiface.IAMAPI, client client.Client, scheme *runtime.Scheme) error {
	secret, err := getIamK8sSecret(cr, client)
	if err != nil {
		if apierror.IsNotFound(err) {
			// clean up access keys if any
			if errDeletingAllAccessKeys := utils.DeleteAllAccessKeys(cr.Spec.IAMUserSpec.Username, iamClient); errDeletingAllAccessKeys != nil {
				return errDeletingAllAccessKeys
			}

			// create fresh access keys
			acccessKeysOutput, errCreatingAccessKeys := utils.CreateAccessKeys(cr.Spec.IAMUserSpec.Username, iamClient)
			if errCreatingAccessKeys != nil {
				return errCreatingAccessKeys
			}

			// create k8s secret
			if errCreatingSecret := createIamK8sSecret(cr,
				*acccessKeysOutput.AccessKey.AccessKeyId,
				*acccessKeysOutput.AccessKey.SecretAccessKey,
				client, scheme); errCreatingSecret != nil {
				return errCreatingSecret
			}
		}
		// if err is something else other then isNotFound, return that error
		return err
	}

	// if secret is found make sure access keys matches the one in IAM
	accessKeyMatches, errCheckingForMatch := secretAccessKeyAndIamAccessKeyMatch(cr, secret, iamClient)
	if errCheckingForMatch != nil {
		return errCheckingForMatch
	}
	if !accessKeyMatches {
		// delete secret to re-initiate
		if err := client.Delete(context.TODO(), secret); err != nil {
			return err
		}
		// return error to force a requeue
		return customErrors.ErrorIAMK8SSecretNeedsUpdate{Message: "AccessKeyId no longer matches with AWS"}
	}

	return nil
}

func CreateOrUpdateIAMPolicy(cr *v1alpha1.S3, iamClient iamiface.IAMAPI) error {
	inlinePolicyIn, errCreatingPolicyInput := cr.GetRestrictedInlinePolicyInput()
	if errCreatingPolicyInput != nil {
		return errCreatingPolicyInput
	}

	_, errCreatingPolicy := iamClient.PutUserPolicy(inlinePolicyIn)
	if errCreatingPolicy != nil {
		return errCreatingPolicy
	}

	return nil

}
