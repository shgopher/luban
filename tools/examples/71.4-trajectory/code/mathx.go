package main

import "math"

func sin(x float64) float64          { return math.Sin(x) }
func cos(x float64) float64          { return math.Cos(x) }
func latRad(lat float64) float64     { return lat * math.Pi / 180 }
func mathAtan2(y, x float64) float64 { return math.Atan2(y, x) }
