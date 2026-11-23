package main

import (
	"encoding/binary"
	"strconv"
)

func appendUvarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func readUvarint(b []byte, i *int) uint64 {
	var v uint64
	var s uint
	for {
		c := b[*i]
		*i++
		v |= uint64(c&0x7f) << s
		if c < 0x80 {
			return v
		}
		s += 7
	}
}

func zigzag(v int64) uint64   { return uint64(v<<1) ^ uint64(v>>63) }
func unzigzag(u uint64) int64 { return int64(u>>1) ^ -int64(u&1) }

// ① CSV 文本：ts,lng(7 位),lat(7 位)
func encCSV(pts []Pt) []byte {
	b := make([]byte, 0, len(pts)*32)
	for _, p := range pts {
		b = strconv.AppendInt(b, p.TS, 10)
		b = append(b, ',')
		b = strconv.AppendFloat(b, p.Lng, 'f', 7, 64)
		b = append(b, ',')
		b = strconv.AppendFloat(b, p.Lat, 'f', 7, 64)
		b = append(b, '\n')
	}
	return b
}

func decCSV(b []byte) []Pt {
	out := make([]Pt, 0, len(b)/32+1)
	i := 0
	for i < len(b) {
		j := i
		for b[j] != ',' {
			j++
		}
		ts, _ := strconv.ParseInt(string(b[i:j]), 10, 64)
		k := j + 1
		for b[k] != ',' {
			k++
		}
		lng, _ := strconv.ParseFloat(string(b[j+1:k]), 64)
		m := k + 1
		for b[m] != '\n' {
			m++
		}
		lat, _ := strconv.ParseFloat(string(b[k+1:m]), 64)
		out = append(out, Pt{TS: ts, Lng: lng, Lat: lat})
		i = m + 1
	}
	return out
}

// ② 二进制：int64 ts + float64 lng + float64 lat = 24 B/点
func encBin(pts []Pt) []byte {
	b := make([]byte, 24*len(pts))
	for i, p := range pts {
		binary.LittleEndian.PutUint64(b[i*24:], uint64(p.TS))
		binary.LittleEndian.PutUint64(b[i*24+8:], uint64(float64bits(p.Lng)))
		binary.LittleEndian.PutUint64(b[i*24+16:], uint64(float64bits(p.Lat)))
	}
	return b
}

func decBin(b []byte) []Pt {
	out := make([]Pt, len(b)/24)
	for i := range out {
		out[i] = Pt{
			TS:  int64(binary.LittleEndian.Uint64(b[i*24:])),
			Lng: mathFromBits(binary.LittleEndian.Uint64(b[i*24+8:])),
			Lat: mathFromBits(binary.LittleEndian.Uint64(b[i*24+16:])),
		}
	}
	return out
}

// ③/④ delta-ts(varint) + zigzag-delta 坐标(varint, 量化 scale 分之一度)
func encDelta(pts []Pt, scale float64) []byte {
	b := make([]byte, 0, len(pts)*6)
	prevTS := int64(0)
	var prevLng, prevLat int64
	for i, p := range pts {
		lngQ := int64(p.Lng * scale)
		latQ := int64(p.Lat * scale)
		if i == 0 {
			b = appendUvarint(b, uint64(p.TS))
			b = appendUvarint(b, zigzag(lngQ))
			b = appendUvarint(b, zigzag(latQ))
		} else {
			b = appendUvarint(b, uint64(p.TS-prevTS))
			b = appendUvarint(b, zigzag(lngQ-prevLng))
			b = appendUvarint(b, zigzag(latQ-prevLat))
		}
		prevTS, prevLng, prevLat = p.TS, lngQ, latQ
	}
	return b
}

func decDelta(b []byte, scale float64) []Pt {
	out := make([]Pt, 0, len(b)/4+1)
	i := 0
	prevTS := int64(0)
	var prevLng, prevLat int64
	for i < len(b) {
		ts := readUvarint(b, &i)
		if len(out) == 0 {
			prevTS = int64(ts)
			prevLng = unzigzag(readUvarint(b, &i))
			prevLat = unzigzag(readUvarint(b, &i))
		} else {
			prevTS += int64(ts)
			prevLng += unzigzag(readUvarint(b, &i))
			prevLat += unzigzag(readUvarint(b, &i))
		}
		out = append(out, Pt{TS: prevTS, Lng: float64(prevLng) / scale, Lat: float64(prevLat) / scale})
	}
	return out
}

func encPolylineValue(v int64, b []byte) []byte {
	v = v << 1
	if v < 0 {
		v = ^v
	}
	for v >= 0x20 {
		b = append(b, byte((0x20|(v&0x1f))+63))
		v >>= 5
	}
	return append(b, byte(v+63))
}

func decPolylineValue(b []byte, i *int) int64 {
	var result int64
	var shift uint
	for {
		c := int64(b[*i]) - 63
		*i++
		result |= (c & 0x1f) << shift
		shift += 5
		if c < 0x20 {
			break
		}
	}
	if result&1 == 1 {
		return ^(result >> 1)
	}
	return result >> 1
}

// ⑤ Google Polyline：坐标 delta 1e-5，不含时间戳
func encPolyline(pts []Pt) []byte {
	b := make([]byte, 0, len(pts)*4)
	var prevLat, prevLng int64
	for _, p := range pts {
		lat := int64(p.Lat * 1e5)
		lng := int64(p.Lng * 1e5)
		b = encPolylineValue(lat-prevLat, b)
		b = encPolylineValue(lng-prevLng, b)
		prevLat, prevLng = lat, lng
	}
	return b
}

func decPolyline(b []byte) []Pt {
	out := make([]Pt, 0, len(b)/3+1)
	i := 0
	var lat, lng int64
	for i < len(b) {
		lat += decPolylineValue(b, &i)
		lng += decPolylineValue(b, &i)
		out = append(out, Pt{Lng: float64(lng) / 1e5, Lat: float64(lat) / 1e5})
	}
	return out
}
