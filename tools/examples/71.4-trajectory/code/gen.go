package main

import "math/rand"

const (
	baseLng = 116.48105
	baseLat = 39.996794
)

// 城市道路随机游走：随机选路点驶向它，途中带航向噪声与限速（0–50 km/h），
// 到点后重新选路点（形成路口转弯）；采样 1–5 s；GPS 噪声高斯 sigma 米。
func genTrajectory(n int, seed int64, startLng, startLat, boxM, noiseM float64) []Pt {
	r := rand.New(rand.NewSource(seed))
	pts := make([]Pt, n)
	lng, lat := startLng, startLat
	pick := func() (float64, float64) {
		return startLng + (r.Float64()*2-1)*boxM/(111320*0.766),
			startLat + (r.Float64()*2-1)*boxM/110540
	}
	wLng, wLat := pick()
	heading := mathAtan2(wLat-lat, (wLng-lng)*0.766)
	speed := 0.0
	ts := int64(1793510400)
	for i := 0; i < n; i++ {
		dt := float64(r.Intn(5) + 1)
		speed += r.NormFloat64() * 0.6
		if speed < 0 {
			speed = 0
		}
		if speed > 13.9 {
			speed = 13.9
		}
		dx := (wLng - lng) * 111320 * cos(latRad(baseLat))
		dy := (wLat - lat) * 110540
		if dx*dx+dy*dy < 80*80 {
			wLng, wLat = pick()
			dx = (wLng - lng) * 111320 * cos(latRad(baseLat))
			dy = (wLat - lat) * 110540
		}
		desired := mathAtan2(dy, dx)
		dh := desired - heading
		for dh > 3.141592653589793 {
			dh -= 2 * 3.141592653589793
		}
		for dh < -3.141592653589793 {
			dh += 2 * 3.141592653589793
		}
		heading += dh*0.18 + r.NormFloat64()*0.06
		step := speed * dt
		ts += int64(dt)
		lat += step * cos(heading) / 110540
		lng += step * sin(heading) / (111320 * cos(latRad(baseLat)))
		// 硬边界：不越出 2 倍盒子（防止航向噪声把轨迹带飞）
		if (lat-startLat)*110540 > 2*boxM || (lat-startLat)*110540 < -2*boxM ||
			(lng-startLng)*111320*cos(latRad(baseLat)) > 2*boxM || (lng-startLng)*111320*cos(latRad(baseLat)) < -2*boxM {
			lat, lng = startLat, startLng
			wLng, wLat = pick()
		}
		pts[i] = Pt{
			TS:  ts,
			Lng: lng + r.NormFloat64()*noiseM/(111320*cos(latRad(baseLat))),
			Lat: lat + r.NormFloat64()*noiseM/110540,
		}
	}
	return pts
}

// 静止 + GPS 白噪声：1 Hz，n 个点
func genStationary(n int, seed int64, sigmaM float64) ([]Pt, []float64) {
	r := rand.New(rand.NewSource(seed))
	pts := make([]Pt, n)
	acc := make([]float64, n)
	ts := int64(1793510400)
	for i := 0; i < n; i++ {
		ts++
		lng := baseLng + r.NormFloat64()*sigmaM/(111320*cos(latRad(baseLat)))
		lat := baseLat + r.NormFloat64()*sigmaM/110540
		pts[i] = Pt{TS: ts, Lng: lng, Lat: lat}
		// 上报精度：约 1.5 sigma 的量级，2% 的多径离群点给 60–150 m
		a := 1.5 * sigmaM * (0.5 + r.Float64())
		if r.Float64() < 0.02 {
			a = 60 + r.Float64()*90
		}
		acc[i] = a
	}
	return pts, acc
}
