package config

var BLS12_377 = Curve{
	Name:         "bls12-377",
	CurvePackage: "bls12377",
	EnumID:       "BLS12_377",
	FrModulus:    "8444461749428370424248824938781546531375899335154063827935233455917409239041",
	FpModulus:    "258664426012969094010652733694893533536393512754914660539884262666720468348340822774968888139573360124440321458177",
	G1: Point{
		CoordType:        "fp.Element",
		PointName:        "g1",
		GLV:              true,
		CofactorCleaning: true,
		CRange:           defaultCRange(),
	},
	G2: Point{
		CoordType:        "fptower.E2",
		PointName:        "g2",
		GLV:              true,
		CofactorCleaning: true,
		CRange:           defaultCRange(),
		Projective:       true,
	},
}

func init() {
	addCurve(&BLS12_377)

}
