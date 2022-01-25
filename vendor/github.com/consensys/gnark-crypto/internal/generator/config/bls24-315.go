package config

var BLS24_315 = Curve{
	Name:         "bls24-315",
	CurvePackage: "bls24315",
	EnumID:       "BLS24_315",
	FrModulus:    "11502027791375260645628074404575422495959608200132055716665986169834464870401",
	FpModulus:    "39705142709513438335025689890408969744933502416914749335064285505637884093126342347073617133569",
	G1: Point{
		CoordType:        "fp.Element",
		PointName:        "g1",
		GLV:              true,
		CofactorCleaning: true,
		CRange:           defaultCRange(),
	},
	G2: Point{
		CoordType:        "fptower.E4",
		PointName:        "g2",
		GLV:              true,
		CofactorCleaning: true,
		CRange:           defaultCRange(),
		Projective:       true,
	},
}

func init() {
	addCurve(&BLS24_315)

}
