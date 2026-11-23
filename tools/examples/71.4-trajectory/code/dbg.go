package main

import "fmt"

func dbg() {
	const n = 1000000
	pts := genTrajectory(n, 42, baseLng, baseLat, 6000, 3)
	keep := onlineKeep(pts, 2)
	simp := keptPts(pts, keep)
	fmt.Println("kept", len(simp), "of", n)
	j := 0
	maxD := 0.0
	maxI := 0
	for i := 1; i < len(pts)-1; i++ {
		for j+1 < len(simp) && simp[j+1].TS <= pts[i].TS {
			j++
		}
		if j+1 >= len(simp) {
			j = len(simp) - 2
		}
		d := perpDist(pts[i], simp[j], simp[j+1])
		if d > maxD {
			maxD, maxI = d, i
		}
	}
	fmt.Println("maxPerp", maxD, "at", maxI, "j", j)
	fmt.Printf("point %d ts=%d lng=%.7f lat=%.7f\n", maxI, pts[maxI].TS, pts[maxI].Lng, pts[maxI].Lat)
	jj := 0
	for jj+1 < len(simp) && simp[jj+1].TS <= pts[maxI].TS {
		jj++
	}
	fmt.Printf("seg a ts=%d lng=%.7f lat=%.7f b ts=%d lng=%.7f lat=%.7f\n", simp[jj].TS, simp[jj].Lng, simp[jj].Lat, simp[jj+1].TS, simp[jj+1].Lng, simp[jj+1].Lat)
	fmt.Println("dist a-b", haversine(simp[jj].Lng, simp[jj].Lat, simp[jj+1].Lng, simp[jj+1].Lat))
}
