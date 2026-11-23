package main

import "math"

func float64bits(f float64) uint64  { return math.Float64bits(f) }
func mathFromBits(u uint64) float64 { return math.Float64frombits(u) }
