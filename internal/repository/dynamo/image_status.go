package dynamo

import (
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// statusIs matches an image whose status attribute is from. Images stored
// before moderation have no status and read as approved, so approved also
// matches a missing one.
func statusIs(status expression.NameBuilder, from repository.ImageStatus) expression.ConditionBuilder {
	cond := status.Equal(expression.Value(from))
	if from == repository.ImageStatusApproved {
		cond = cond.Or(expression.AttributeNotExists(status))
	}
	return cond
}

// statusUpdate sets statusName to to, and reasonName to reason or removes
// it.
func statusUpdate(statusName, reasonName expression.NameBuilder, to repository.ImageStatus, reason *string) expression.UpdateBuilder {
	return setOrRemoveName(expression.Set(statusName, expression.Value(to)), reasonName, reason)
}
