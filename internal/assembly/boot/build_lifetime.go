package boot

import "context"

// constructionLifetime relays request cancellation until successful assembly;
// the controller's cleanup owns the detached lifetime afterward.
func constructionLifetime(request context.Context) (context.Context, func() error, context.CancelFunc) {
	life, cancel := context.WithCancel(context.WithoutCancel(request))
	stop := context.AfterFunc(request, cancel)
	detach := func() error {
		if !stop() || request.Err() != nil {
			cancel()
			return request.Err()
		}
		return nil
	}
	return life, detach, func() { stop(); cancel() }
}
