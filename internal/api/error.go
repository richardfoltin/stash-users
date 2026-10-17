package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func gqlErrorHandler(ctx context.Context, e error) *gqlerror.Error {
	if !errors.Is(ctx.Err(), context.Canceled) {
		// log all errors - for now just log the error message
		// we can potentially add more context later
		fc := graphql.GetFieldContext(ctx)
		if fc != nil {
			if isPermissionDenied(e) {
				logger.Warnf("%s: %v", fc.Path(), e)
			} else {
				logger.Errorf("%s: %v", fc.Path(), e)
			}

			// log the args in debug level, without credentials
			logger.DebugFunc(func() (string, []interface{}) {
				var args interface{}
				args = fc.Args

				s, _ := json.Marshal(args)
				if len(s) > 0 {
					args = string(redactArgsJSON(s))
				}

				return "%s: %v", []interface{}{
					fc.Path(),
					args,
				}
			})
		}
	}

	// we may also want to transform the error message for the response
	// for now just return the original error
	return graphql.DefaultErrorPresenter(ctx, e)
}

// redactedValue replaces the values of redacted arguments.
const redactedValue = "[redacted]"

// isCredentialKey returns true if the argument with the key may contain a
// credential, such as a password, an API key or the Handy connection key.
func isCredentialKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "password") ||
		strings.Contains(k, "apikey") ||
		strings.Contains(k, "api_key") ||
		strings.Contains(k, "handykey")
}

// redactArgs replaces the values of credential arguments, at any depth.
func redactArgs(v interface{}) interface{} {
	switch vv := v.(type) {
	case map[string]interface{}:
		for k, val := range vv {
			if isCredentialKey(k) {
				vv[k] = redactedValue
			} else {
				vv[k] = redactArgs(val)
			}
		}
	case []interface{}:
		for i, val := range vv {
			vv[i] = redactArgs(val)
		}
	}

	return v
}

// redactArgsJSON redacts the credential arguments of the JSON encoded
// arguments. If the arguments cannot be decoded, nothing is returned.
func redactArgsJSON(data []byte) []byte {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil
	}

	ret, err := json.Marshal(redactArgs(v))
	if err != nil {
		return nil
	}

	return ret
}
