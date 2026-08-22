package lua

const preloadLimit = 256
const intPreloadLimit = 65536 // Common loop bounds

var preloadedNumbers [preloadLimit]LValue
var preloadedIntegers [intPreloadLimit * 2]LValue // [-65536, 65536)

func init() {
	for i := 0; i < preloadLimit; i++ {
		preloadedNumbers[i] = LNumber(i)
	}
	for i := -intPreloadLimit; i < intPreloadLimit; i++ {
		preloadedIntegers[i+intPreloadLimit] = LInteger(i)
	}
}

func lnumberToValue(v LNumber) LValue {
	iv := int(v)
	if iv >= 0 && iv < preloadLimit && LNumber(iv) == v {
		return preloadedNumbers[iv]
	}
	return v
}

func lintegerToValue(v LInteger) LValue {
	iv := int(v)
	if iv >= -intPreloadLimit && iv < intPreloadLimit {
		return preloadedIntegers[iv+intPreloadLimit]
	}
	return v
}
