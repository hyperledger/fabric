package config

var BW6_761 = Curve{
	Name:         "bw6-761",
	CurvePackage: "bw6761",
	EnumID:       "BW6_761",
	FrModulus:    "258664426012969094010652733694893533536393512754914660539884262666720468348340822774968888139573360124440321458177",
	FpModulus:    "6891450384315732539396789682275657542479668912536150109513790160209623422243491736087683183289411687640864567753786613451161759120554247759349511699125301598951605099378508850372543631423596795951899700429969112842764913119068299",
	G1: Point{
		CoordType:        "fp.Element",
		PointName:        "g1",
		GLV:              true,
		CofactorCleaning: true,
		CRange:           []int{4, 5, 8, 16},
		Projective:       true,
	},
	G2: Point{
		CoordType:        "fp.Element",
		PointName:        "g2",
		GLV:              true,
		CofactorCleaning: true,
		CRange:           []int{4, 5, 8, 16},
	},
}

func init() {
	addCurve(&BW6_761)
}
