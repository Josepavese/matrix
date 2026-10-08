package runapi

import "github.com/Josepavese/matrix/internal/middleware"

func requestedCapacity(req runRequest) middleware.CapacityRequest {
	if req.Capacity == nil {
		return middleware.CapacityRequest{}
	}
	return *req.Capacity
}
