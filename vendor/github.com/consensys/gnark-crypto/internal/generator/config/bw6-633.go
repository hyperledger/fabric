package config

var BW6_633 = Curve{
	Name:         "bw6-633",
	CurvePackage: "bw6633",
	EnumID:       "BW6_633",
	FrModulus:    "39705142709513438335025689890408969744933502416914749335064285505637884093126342347073617133569",
	FpModulus:    "20494478644167774678813387386538961497669590920908778075528754551012016751717791778743535050360001387419576570244406805463255765034468441182772056330021723098661967429339971741066259394985997",
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
	addCurve(&BW6_633)
}
