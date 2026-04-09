package db

import "math"

// RoundMoney normalizes currency values to two decimal places.
func RoundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
