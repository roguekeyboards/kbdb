// Package deletenotice tells the operator an account was deleted, so they
// can remove the user from the identity provider by hand: mykeebs-api holds no
// credential that could do it (see #554).
package deletenotice

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

type snsAPI interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

// SNS publishes each notice to an SNS topic the operator subscribes to.
type SNS struct {
	client   snsAPI
	topicARN string
}

// NewSNS returns an SNS publishing to topicARN.
func NewSNS(client *sns.Client, topicARN string) *SNS {
	return &SNS{client: client, topicARN: topicARN}
}

// Notify implements accountdelete.Notifier. The notice carries only the
// user ID, which is the identity provider's own ID for the user.
func (s *SNS) Notify(ctx context.Context, userID string) error {
	_, err := s.client.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(s.topicARN),
		Subject:  aws.String("mykeebs-api: account deleted, remove the user from the identity provider"),
		Message: aws.String(fmt.Sprintf(
			"Account %s was deleted from mykeebs-api. Remove this user from the identity provider.", userID)),
	})
	if err != nil {
		return fmt.Errorf("publishing account delete notice to %s: %w", s.topicARN, err)
	}
	return nil
}
