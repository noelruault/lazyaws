// Read-only is the default, and this is where that is enforced rather than promised.
package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/aws/smithy-go/middleware"
)

// writesAllowed is process wide and starts closed, so the absence of --allow-writes denies rather than permits.
// It is not a field on Client because the gate is a property of the process: clients are rebuilt on every profile switch and on the cached-credentials path, and a per-client flag would be one constructor away from being forgotten.
var writesAllowed atomic.Bool

// ErrReadOnly is what every refused AWS call unwraps to, so callers can tell a policy refusal from a permissions error and say something different about it.
var ErrReadOnly = errors.New("lazyaws is read-only")

// AllowWrites opens the gate for the rest of the process. Only main calls it, and only when the flag was given.
func AllowWrites() {
	writesAllowed.Store(true)
}

// WritesAllowed reports whether mutating calls will be let through, so the UI can hide what it would only refuse later.
func WritesAllowed() bool {
	return writesAllowed.Load()
}

// requireWrites is for the things this package does without the SDK: spawning a terminal for an SSM session, forwarding a port, rewriting the local kubeconfig.
// None of those pass through the middleware, so they ask here rather than trusting that whoever called them checked. Each is exported, and an exported function that executes something has to carry its own gate.
func requireWrites(what string) error {
	if writesAllowed.Load() {
		return nil
	}

	return fmt.Errorf("%w: refused %s; restart with --allow-writes to permit it", ErrReadOnly, what)
}

// readVerbs are the prefixes AWS gives operations that only read. Anything outside them is treated as a mutation, because the cost of guessing wrong in this direction is a refused read rather than a resource that changed.
var readVerbs = []string{"Describe", "Get", "List", "Head", "BatchGet", "Lookup", "Filter", "Search"}

// readOperations are the reads AWS did not name after a read verb, allowed by exact name.
var readOperations = map[string]bool{
	// Credential resolution, not account access: a profile with role_arn assumes a role on every refresh, and an SSO session mints and refreshes a token. Refusing these would refuse the login rather than the writes.
	"AssumeRole":                true,
	"AssumeRoleWithWebIdentity": true,
	"AssumeRoleWithSAML":        true,
	"CreateToken":               true,
	"RegisterClient":            true,
	"StartDeviceAuthorization":  true,

	// Model inference answers a question and creates nothing. The chat is off until switched on, and its Kiro backend, which really does run AWS commands, is refused separately because it never reaches this stack.
	"Converse":                      true,
	"ConverseStream":                true,
	"InvokeModel":                   true,
	"InvokeModelWithResponseStream": true,

	// A live log tail is a read that AWS named Start.
	"StartLiveTail": true,
}

// readOperation decides whether one AWS operation may run while writes are denied.
// An empty name is refused: a guard that cannot tell what it is about to allow has to fail closed.
func readOperation(operation string) bool {
	if operation == "" {
		return false
	}
	if readOperations[operation] {
		return true
	}

	for _, verb := range readVerbs {
		if strings.HasPrefix(operation, verb) {
			return true
		}
	}

	return false
}

// readOnlyGuard refuses a mutating operation in the Initialize step, which runs before the request is serialized, signed or sent, so a refused call never reaches AWS at all.
// It is attached to every client unconditionally and reads the gate per request: a client built before the flag was applied cannot outlive the policy, and there is no constructor that can produce a client without it.
func readOnlyGuard(stack *middleware.Stack) error {
	return stack.Initialize.Add(middleware.InitializeMiddlewareFunc(
		"lazyawsReadOnlyGuard",
		func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
			if writesAllowed.Load() {
				return next.HandleInitialize(ctx, in)
			}

			// Both values are put on the context by invokeOperation before the stack runs, so they are readable here whatever order the middlewares were registered in.
			service, operation := middleware.GetServiceID(ctx), middleware.GetOperationName(ctx)
			if readOperation(operation) {
				return next.HandleInitialize(ctx, in)
			}

			return middleware.InitializeOutput{}, middleware.Metadata{}, fmt.Errorf("%w: refused %s %s; restart with --allow-writes to permit it", ErrReadOnly, service, operation)
		},
	), middleware.Before)
}
