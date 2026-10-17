package api

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	hasPermissionDirectiveName = "hasPermission"
	hasPermissionArgument      = "permission"
	adminOnlyDirectiveName     = "adminOnly"
	signedInDirectiveName      = "signedIn"

	errCodePermissionDenied = "PERMISSION_DENIED"
)

// requirement is what a field requires of the current user: a permission,
// the Admin role, or only being signed in.
type requirement struct {
	permission models.Permission
	adminOnly  bool
	signedIn   bool
}

// mutations without a directive are restricted to the Admin role
var adminRequirement = requirement{adminOnly: true}

func newPermissionDeniedError(req requirement) error {
	if req.adminOnly {
		return &gqlerror.Error{
			Message: "permission denied: this action requires the Admin role",
			Extensions: map[string]interface{}{
				"code":         errCodePermissionDenied,
				"requiredRole": models.RoleIDAdmin,
			},
		}
	}

	return &gqlerror.Error{
		Message: fmt.Sprintf("permission denied: this action requires the %s permission", req.permission),
		Extensions: map[string]interface{}{
			"code":               errCodePermissionDenied,
			"requiredPermission": req.permission.String(),
		},
	}
}

func isPermissionDenied(err error) bool {
	var gqlErr *gqlerror.Error
	return errors.As(err, &gqlErr) && gqlErr.Extensions["code"] == errCodePermissionDenied
}

// checkRequirement returns an error if the current user does not meet the
// requirement.
func checkRequirement(ctx context.Context, req requirement) error {
	currentUser := models.GetCurrentUser(ctx)

	// if there is no current user, this is an anonymous request
	// we should not end up here unless there are no credentials required
	if currentUser == nil {
		return nil
	}

	switch {
	case req.signedIn:
		return nil
	case req.adminOnly:
		if currentUser.IsAdmin() {
			return nil
		}
	case currentUser.HasPermission(req.permission):
		return nil
	}

	return newPermissionDeniedError(req)
}

// isUnset returns true for nil and false values.
func isUnset(v interface{}) bool {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return true
		}
		rv = rv.Elem()
	}

	return !rv.IsValid() || (rv.Kind() == reflect.Bool && !rv.Bool())
}

// checkField checks the requirement of a field before resolving it. An input
// field is resolved first, and checked only if it is set: false and null
// values are allowed.
func checkField(ctx context.Context, obj interface{}, next graphql.Resolver, req requirement) (interface{}, error) {
	// the object of an input field is the input map
	if _, inputField := obj.(map[string]interface{}); inputField {
		v, err := next(ctx)
		if err != nil || isUnset(v) {
			return v, err
		}

		if err := checkRequirement(ctx, req); err != nil {
			return nil, err
		}

		return v, nil
	}

	if err := checkRequirement(ctx, req); err != nil {
		return nil, err
	}

	return next(ctx)
}

func HasPermissionDirective(ctx context.Context, obj interface{}, next graphql.Resolver, permission models.Permission) (interface{}, error) {
	return checkField(ctx, obj, next, requirement{permission: permission})
}

func AdminOnlyDirective(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	return checkField(ctx, obj, next, adminRequirement)
}

func SignedInDirective(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	return checkField(ctx, obj, next, requirement{signedIn: true})
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
	if !currentUser.IsAdmin() && currentUser.Username != userObj.Username {
		return nil, newPermissionDeniedError(adminRequirement)
	}

	return next(ctx)
}

// fieldRequirement returns the requirement of the directive of the field, if
// it has one.
func fieldRequirement(def *ast.FieldDefinition) (requirement, bool) {
	if def == nil {
		return requirement{}, false
	}

	if d := def.Directives.ForName(hasPermissionDirectiveName); d != nil {
		arg := d.Arguments.ForName(hasPermissionArgument)
		if arg == nil || arg.Value == nil {
			return requirement{}, false
		}

		return requirement{permission: models.Permission(arg.Value.Raw)}, true
	}

	if def.Directives.ForName(adminOnlyDirectiveName) != nil {
		return adminRequirement, true
	}

	if def.Directives.ForName(signedInDirectiveName) != nil {
		return requirement{signedIn: true}, true
	}

	return requirement{}, false
}

// mutationRequirement returns the requirement of the mutation: that of its
// directive, or the Admin role.
func mutationRequirement(def *ast.FieldDefinition) requirement {
	req, ok := fieldRequirement(def)
	if ok && (req.adminOnly || req.signedIn || req.permission.IsValid()) {
		return req
	}

	return adminRequirement
}

func isMutation(ctx context.Context) bool {
	if !graphql.HasOperationContext(ctx) {
		return false
	}

	op := graphql.GetOperationContext(ctx).Operation
	return op != nil && op.Operation == ast.Mutation
}

// mutationRequirementMiddleware checks the requirement of mutations, which is
// the Admin role for mutations without a directive. Mutations are identified
// by the operation type.
func mutationRequirementMiddleware(ctx context.Context, next graphql.RootResolver) graphql.Marshaler {
	rfc := graphql.GetRootFieldContext(ctx)
	if rfc != nil && rfc.Field.Field != nil && isMutation(ctx) {
		if err := checkRequirement(ctx, mutationRequirement(rfc.Field.Definition)); err != nil {
			graphql.AddError(ctx, err)
			return graphql.Null
		}
	}

	return next(ctx)
}
