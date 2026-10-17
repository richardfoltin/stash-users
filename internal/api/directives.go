package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	hasRoleDirectiveName = "hasRole"
	hasRoleRoleArgument  = "role"

	// mutations without a @hasRole directive are denied by default
	defaultMutationRole = models.RoleEnumAdmin

	errCodePermissionDenied = "PERMISSION_DENIED"
)

func newPermissionDeniedError(role models.RoleEnum) error {
	return &gqlerror.Error{
		Message: fmt.Sprintf("permission denied: this action requires the %s role", role),
		Extensions: map[string]interface{}{
			"code":         errCodePermissionDenied,
			"requiredRole": role.String(),
		},
	}
}

func isPermissionDenied(err error) bool {
	var gqlErr *gqlerror.Error
	return errors.As(err, &gqlErr) && gqlErr.Extensions["code"] == errCodePermissionDenied
}

// checkRole returns an error if the current user does not have the role.
func checkRole(ctx context.Context, role models.RoleEnum) error {
	currentUser := models.GetCurrentUser(ctx)

	// if there is no current user, this is an anonymous request
	// we should not end up here unless there are no credentials required
	if currentUser == nil {
		return nil
	}

	if !currentUser.Roles.HasRole(role) {
		return newPermissionDeniedError(role)
	}

	return nil
}

func HasRoleDirective(ctx context.Context, obj interface{}, next graphql.Resolver, role models.RoleEnum) (interface{}, error) {
	if err := checkRole(ctx, role); err != nil {
		return nil, err
	}

	return next(ctx)
}

func IsUserOwnerDirective(ctx context.Context, obj any, next graphql.Resolver) (res any, err error) {
	currentUser := models.GetCurrentUser(ctx)

	// if there is no current user, this is an anonymous request
	// we should not end up here unless there are no credentials required
	if currentUser == nil {
		return next(ctx)
	}

	// get the user from the object
	userObj, ok := obj.(*models.User)
	if !ok {
		return nil, session.ErrUnauthorized
	}

	// allow admin access
	if !currentUser.Roles.HasRole(models.RoleEnumAdmin) && currentUser.Username != userObj.Username {
		return nil, newPermissionDeniedError(models.RoleEnumAdmin)
	}

	return next(ctx)
}

// fieldRole returns the role of the @hasRole directive of the field, if any.
func fieldRole(def *ast.FieldDefinition) (models.RoleEnum, bool) {
	if def == nil {
		return "", false
	}

	d := def.Directives.ForName(hasRoleDirectiveName)
	if d == nil {
		return "", false
	}

	arg := d.Arguments.ForName(hasRoleRoleArgument)
	if arg == nil || arg.Value == nil {
		return "", false
	}

	return models.RoleEnum(arg.Value.Raw), true
}

// mutationRole returns the role required to run the mutation.
func mutationRole(def *ast.FieldDefinition) models.RoleEnum {
	if role, ok := fieldRole(def); ok && role.IsValid() {
		return role
	}

	return defaultMutationRole
}

func isMutation(ctx context.Context) bool {
	if !graphql.HasOperationContext(ctx) {
		return false
	}

	op := graphql.GetOperationContext(ctx).Operation
	return op != nil && op.Operation == ast.Mutation
}

// mutationRoleMiddleware requires the ADMIN role for mutations without a
// @hasRole directive. Mutations are identified by the operation type.
func mutationRoleMiddleware(ctx context.Context, next graphql.RootResolver) graphql.Marshaler {
	rfc := graphql.GetRootFieldContext(ctx)
	if rfc != nil && rfc.Field.Field != nil && isMutation(ctx) {
		if err := checkRole(ctx, mutationRole(rfc.Field.Definition)); err != nil {
			graphql.AddError(ctx, err)
			return graphql.Null
		}
	}

	return next(ctx)
}
