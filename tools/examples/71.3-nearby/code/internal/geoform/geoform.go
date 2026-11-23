// Package geoform 实现「附近的人」用到的四种距离算法 + WGS84 椭球真值。
// 所有角度参数为十进制度（lat, lng），距离单位为米。
package geoform

import (
	"math"

	"github.com/golang/geo/s2"
)

const (
	// EarthRadius 是全书统一口径的球面半径：6371 km。
	EarthRadius = 6371000.0
	// WGS84 椭球参数。
	WGS84A = 6378137.0
	WGS84F = 1.0 / 298.257223563
	WGS84B = WGS84A * (1 - WGS84F)
)

func rad(d float64) float64 { return d * math.Pi / 180 }
func deg(r float64) float64 { return r * 180 / math.Pi }

// Haversine 球面距离（生产公式），R=6371 km。
func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
	p1, p2 := rad(lat1), rad(lat2)
	dp := p2 - p1
	dl := rad(lng2 - lng1)
	sinDP := math.Sin(dp / 2)
	sinDL := math.Sin(dl / 2)
	h := sinDP*sinDP + math.Cos(p1)*math.Cos(p2)*sinDL*sinDL
	return 2 * EarthRadius * math.Asin(math.Min(1, math.Sqrt(h)))
}

// Equirectangular 等距圆柱近似（含 cos 纬度修正）。
func Equirectangular(lat1, lng1, lat2, lng2 float64) float64 {
	meanLat := rad((lat1 + lat2) / 2)
	x := rad(lng2-lng1) * math.Cos(meanLat)
	y := rad(lat2 - lat1)
	return EarthRadius * math.Hypot(x, y)
}

// NaiveDegrees 错误示范：直接把经纬度差当平面距离，1 度 = 111 320 m。
func NaiveDegrees(lat1, lng1, lat2, lng2 float64) float64 {
	const metersPerDegree = 111320.0
	dLat := lat2 - lat1
	dLng := lng2 - lng1
	return metersPerDegree * math.Hypot(dLat, dLng)
}

// S2Spherical 用 golang/geo/s2 的 LatLng.Distance（球面角距离）换算成米。
// 注意：这不是椭球距离，是球面角距离；用来验证 haversine 实现的量级。
func S2Spherical(lat1, lng1, lat2, lng2 float64) float64 {
	a := s2.LatLngFromDegrees(lat1, lng1)
	b := s2.LatLngFromDegrees(lat2, lng2)
	return a.Distance(b).Radians() * EarthRadius
}

// VincentyInverse 是 WGS84 椭球测地线距离（反解），本文件里的「真值」。
// 迭代到 1e-12 rad 或 200 次，基本毫米级。
func VincentyInverse(lat1, lng1, lat2, lng2 float64) float64 {
	a, b, f := WGS84A, WGS84B, WGS84F
	L := rad(lng2 - lng1)
	U1 := math.Atan((1 - f) * math.Tan(rad(lat1)))
	U2 := math.Atan((1 - f) * math.Tan(rad(lat2)))
	sinU1, cosU1 := math.Sin(U1), math.Cos(U1)
	sinU2, cosU2 := math.Sin(U2), math.Cos(U2)
	lambda := L
	var sinSigma, cosSigma, sigma, cos2Alpha, cos2SigmaM float64
	for i := 0; i < 200; i++ {
		sinLambda, cosLambda := math.Sin(lambda), math.Cos(lambda)
		sinSigma = math.Hypot(cosU2*sinLambda, cosU1*sinU2-sinU1*cosU2*cosLambda)
		if sinSigma == 0 {
			return 0
		}
		cosSigma = sinU1*sinU2 + cosU1*cosU2*cosLambda
		sigma = math.Atan2(sinSigma, cosSigma)
		sinAlpha := cosU1 * cosU2 * sinLambda / sinSigma
		cos2Alpha = 1 - sinAlpha*sinAlpha
		if cos2Alpha != 0 {
			cos2SigmaM = cosSigma - 2*sinU1*sinU2/cos2Alpha
		} else {
			cos2SigmaM = 0 // 赤道线
		}
		C := f / 16 * cos2Alpha * (4 + f*(4-3*cos2Alpha))
		lambdaPrev := lambda
		lambda = L + (1-C)*f*sinAlpha*(sigma+C*sinSigma*(cos2SigmaM+C*cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)))
		if math.Abs(lambda-lambdaPrev) < 1e-12 {
			break
		}
	}
	uSq := cos2Alpha * (a*a - b*b) / (b * b)
	A := 1 + uSq/16384*(4096+uSq*(-768+uSq*(320-175*uSq)))
	B := uSq / 1024 * (256 + uSq*(-128+uSq*(74-47*uSq)))
	deltaSigma := B * sinSigma * (cos2SigmaM + B/4*(cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)-
		B/6*cos2SigmaM*(-3+4*sinSigma*sinSigma)*(-3+4*cos2SigmaM*cos2SigmaM)))
	return b * A * (sigma - deltaSigma)
}

// VincentyDirect 是 WGS84 椭球正解：从 (lat,lng) 沿 azimuth（度，正北起顺时针）
// 走 s 米，返回终点。用来造「精确距离为 D」的测试点。
func VincentyDirect(lat, lng, azimuth, s float64) (float64, float64) {
	a, b, f := WGS84A, WGS84B, WGS84F
	alpha1 := rad(azimuth)
	sinAlpha1, cosAlpha1 := math.Sin(alpha1), math.Cos(alpha1)
	tanU1 := (1 - f) * math.Tan(rad(lat))
	cosU1 := 1 / math.Sqrt(1+tanU1*tanU1)
	sinU1 := tanU1 * cosU1
	sigma1 := math.Atan2(tanU1, cosAlpha1)
	sinAlpha := cosU1 * sinAlpha1
	cos2Alpha := 1 - sinAlpha*sinAlpha
	uSq := cos2Alpha * (a*a - b*b) / (b * b)
	A := 1 + uSq/16384*(4096+uSq*(-768+uSq*(320-175*uSq)))
	B := uSq / 1024 * (256 + uSq*(-128+uSq*(74-47*uSq)))
	sigma := s / (b * A)
	var sigmaPrev, cos2SigmaM, sinSigma, cosSigma float64
	for i := 0; i < 100; i++ {
		cos2SigmaM = math.Cos(2*sigma1 + sigma)
		sinSigma = math.Sin(sigma)
		cosSigma = math.Cos(sigma)
		deltaSigma := B * sinSigma * (cos2SigmaM + B/4*(cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)-
			B/6*cos2SigmaM*(-3+4*sinSigma*sinSigma)*(-3+4*cos2SigmaM*cos2SigmaM)))
		sigmaPrev = sigma
		sigma = s/(b*A) + deltaSigma
		if math.Abs(sigma-sigmaPrev) < 1e-12 {
			break
		}
	}
	cos2SigmaM = math.Cos(2*sigma1 + sigma)
	sinSigma = math.Sin(sigma)
	cosSigma = math.Cos(sigma)
	tmp := sinU1*sinSigma - cosU1*cosSigma*cosAlpha1
	lat2 := math.Atan2(sinU1*cosSigma+cosU1*sinSigma*cosAlpha1,
		(1-f)*math.Hypot(sinAlpha, tmp))
	lambda := math.Atan2(sinSigma*sinAlpha1, cosU1*cosSigma-sinU1*sinSigma*cosAlpha1)
	C := f / 16 * cos2Alpha * (4 + f*(4-3*cos2Alpha))
	L := lambda - (1-C)*f*sinAlpha*(sigma+C*sinSigma*(cos2SigmaM+C*cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)))
	lng2 := lng + deg(L)
	// 归一化到 [-180,180]
	for lng2 > 180 {
		lng2 -= 360
	}
	for lng2 < -180 {
		lng2 += 360
	}
	return deg(lat2), lng2
}

// OffsetMeters 把以米为单位的 (北, 东) 偏移加到 WGS84 点上（局部平面近似，
// 仅用于合成测试数据，误差 << 1 cm 量级）。
func OffsetMeters(lat, lng, north, east float64) (float64, float64) {
	dLat := north / 111320.0
	dLng := east / (111320.0 * math.Cos(rad(lat)))
	return lat + dLat, lng + dLng
}
