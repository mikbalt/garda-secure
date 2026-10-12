package service

import "math"

// CheckGPS validates an optional position: both coordinates or none,
// latitude -90..90, longitude -180..180.
func CheckGPS(lat, lng *float64) error {
	if lat == nil && lng == nil {
		return nil
	}
	if lat == nil || lng == nil {
		return BadRequest("invalid_gps", "gps_lat and gps_lng must be sent together").WithField("gps_lat")
	}
	if math.IsNaN(*lat) || *lat < -90 || *lat > 90 {
		return BadRequest("invalid_gps", "gps_lat must be between -90 and 90").WithField("gps_lat")
	}
	if math.IsNaN(*lng) || *lng < -180 || *lng > 180 {
		return BadRequest("invalid_gps", "gps_lng must be between -180 and 180").WithField("gps_lng")
	}
	return nil
}
