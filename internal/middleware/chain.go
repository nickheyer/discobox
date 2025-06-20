// Package middleware provides HTTP middleware implementations
package middleware

import (
	"net/http"
	"discobox/internal/types"
)


// Chain implements the MiddlewareChain interface
type Chain struct {
	middlewares []types.Middleware
}

// NewChain creates a new middleware chain
func NewChain(middlewares ...types.Middleware) types.MiddlewareChain {
	return &Chain{
		middlewares: middlewares,
	}
}

// Use adds middleware to the chain
func (c *Chain) Use(middlewares ...types.Middleware) {
	c.middlewares = append(c.middlewares, middlewares...)
}

// Then creates the final handler by chaining all middleware
func (c *Chain) Then(handler http.Handler) http.Handler {
	// Apply middleware in reverse order so they execute in the order added
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i](handler)
	}
	return handler
}

// Merge combines multiple chains into one
func (c *Chain) Merge(other types.MiddlewareChain) types.MiddlewareChain {
	if otherChain, ok := other.(*Chain); ok {
		return &Chain{
			middlewares: append(c.middlewares, otherChain.middlewares...),
		}
	}
	return c
}

// Clone creates a copy of the chain
func (c *Chain) Clone() types.MiddlewareChain {
	middlewares := make([]types.Middleware, len(c.middlewares))
	copy(middlewares, c.middlewares)
	return &Chain{middlewares: middlewares}
}

// Wrap is a helper to create middleware from a function
func Wrap(fn func(http.Handler) http.Handler) types.Middleware {
	return types.Middleware(fn)
}

// WrapFunc wraps a handler function as middleware
func WrapFunc(fn func(http.ResponseWriter, *http.Request, http.Handler)) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fn(w, r, next)
		})
	}
}

// Conditional applies middleware based on a condition
func Conditional(condition func(*http.Request) bool, middleware types.Middleware) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if condition(r) {
				middleware(next).ServeHTTP(w, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

// Branch creates branching middleware based on a condition
func Branch(condition func(*http.Request) bool, trueMiddleware, falseMiddleware types.Middleware) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if condition(r) {
				trueMiddleware(next).ServeHTTP(w, r)
			} else {
				falseMiddleware(next).ServeHTTP(w, r)
			}
		})
	}
}
