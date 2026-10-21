package network

import (
	"fmt"
	"net/http"
	"strings"
)

// The initial endpoint owns the header values. Neither a different authority
// nor a different scheme may receive them through an HTTP redirect.
func refuseGovernedHeaderRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 0 {
		origin := via[0].URL
		if !strings.EqualFold(req.URL.Scheme, origin.Scheme) || !strings.EqualFold(req.URL.Host, origin.Host) {
			return fmt.Errorf("refusing cross-origin redirect for governed request headers")
		}
	}
	return refuseHTTPSDowngrade(req, via)
}
