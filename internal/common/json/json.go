package localjson

import (
	"fmt"
	"reflect"

	"gitlab.met.no/frost/frost/internal/common"
)

// isJSONSubObjectMatch checks if map1 is a sub-object of map2.
// Returns (false, reason, nil) or (true, "", nil) on success, otherwise (false, "", error).
func isJSONSubObjectMatch(map1, map2 map[string]interface{}) (bool, string, error) {

	for key1, val1 := range map1 {
		val2, found := map2[key1]
		if !found {
			return false, fmt.Sprintf("object key not found: >%s<", key1), nil
		}
		ok, reason, err := IsJSONSubMatch(val1, val2)
		if err != nil {
			return false, "", fmt.Errorf(
				"isJSONSubMatch() failed for map key %s: %v", key1, err)
		}
		if !ok {
			return false, reason, nil
		}
	}

	return true, "", nil
}

// isJSONSubArrayMatch checks if arr1 is a sub-array of arr2.
// The sub-array doesn't have to correspond with a consecutive index range in arr2 but items
// must appear in the same order.
// Returns (false, reason, nil) or (true, "", nil) on success, otherwise (false, "", error).
func isJSONSubArrayMatch(arr1, arr2 []interface{}) (bool, string, error) {

	pos2 := 0 // index in arr2 at which to start looking for a match with an item in arr1
	for i := 0; i < len(arr1); i++ {
		found := false
		for j := pos2; j < len(arr2); j++ {
			ok, _, err := IsJSONSubMatch(arr1[i], arr2[j])
			if err != nil {
				return false, "", fmt.Errorf(
					"isJSONSubMatch() failed for array indices %d and %d: %v", i, j, err)
			}
			if ok { // item i in arr1 accounted for, so proceed to look for a match for item i+1
				found = true
				pos2 = j + 1
				break
			}
		}
		if !found { // arr2 exhausted without all items in arr1 being matched
			return false, fmt.Sprintf(
				"no match found for item %d in array (value: %v)", i, arr1[i]), nil
		}
	}

	return true, "", nil
}

func isKindSupported(kind reflect.Kind) bool {
	supportedKinds := map[reflect.Kind]bool{
		reflect.Struct: true, reflect.Map: true, reflect.Slice: true, reflect.Array: true,
		reflect.Bool: true, reflect.String: true, reflect.Int: true, reflect.Int8: true,
		reflect.Int16: true, reflect.Int32: true, reflect.Int64: true, reflect.Float32: true,
		reflect.Float64: true,
	}
	_, found := supportedKinds[kind]
	return found
}

func isInteger(kind reflect.Kind) bool {
	return (kind == reflect.Int) || (kind == reflect.Int8) || (kind == reflect.Int16) ||
		(kind == reflect.Int32) || (kind == reflect.Int64)
}

func isFloat(kind reflect.Kind) bool {
	return (kind == reflect.Float32) || (kind == reflect.Float64)
}

// IsJSONSubMatch checks if val1 matches a substructure of val2 (e.g. val1={"a1": {"b": "c*"}}
// matches val2={"a1": {"b": "cdef"}, "a2": [1, 2, 3]}).
// Returns (false, reason, nil) or (true, "", nil) on success, otherwise (false, "", error).
func IsJSONSubMatch(val1, val2 interface{}) (bool, string, error) {

	if (val1 == nil) || (val2 == nil) {
		return false, "", fmt.Errorf("val1 and/or val2 is nil:\n\tval1: %v\n\tval2: %v", val1, val2)
	}

	kind1 := reflect.TypeOf(val1).Kind()
	kind2 := reflect.TypeOf(val2).Kind()

	for _, valInfo := range []struct {
		name string
		kind reflect.Kind
	}{{"val1", kind1}, {"val2", kind2}} {
		if !isKindSupported(valInfo.kind) {
			return false, "",
				fmt.Errorf("kind not supported (%s): %s", valInfo.name, valInfo.kind.String())
		}
	}

	switch {
	case ((kind1 == reflect.Struct) || (kind1 == reflect.Map)) &&
		((kind2 == reflect.Struct) || (kind2 == reflect.Map)):
		map1 := val1.(map[string]interface{})
		map2 := val2.(map[string]interface{})
		ok, reason, err := isJSONSubObjectMatch(map1, map2)
		if err != nil {
			return false, "", fmt.Errorf("isJSONSubObjectMatch() failed: %v", err)
		}
		if ok {
			return true, "", nil
		}
		return false, fmt.Sprintf(
			"object\n%v\nnot sub-object of\n%v\nreason: %s", map1, map2, reason), nil

	case ((kind1 == reflect.Slice) || (kind1 == reflect.Array)) &&
		((kind2 == reflect.Slice) || (kind2 == reflect.Array)):
		arr1 := val1.([]interface{})
		arr2 := val2.([]interface{})
		ok, reason, err := isJSONSubArrayMatch(arr1, arr2)
		if err != nil {
			return false, "", fmt.Errorf("isJSONSubArrayMatch() failed: %v", err)
		}
		if ok {
			return true, "", nil
		}
		return false, fmt.Sprintf(
			"array\n%v\nnot sub-array of\n%v\nreason: %s", arr1, arr2, reason), nil

	case (kind1 == reflect.Bool) && (kind2 == reflect.Bool):
		if val1 == val2 {
			return true, "", nil
		}
		return false, fmt.Sprintf("boolean value %v differs from %v", val1, val2), nil

	case (kind1 == reflect.String) && (kind2 == reflect.String):
		s1 := val1.(string) // assuming s1, ok := val1.(string) would have returned ok==true
		s2 := val2.(string) // assuming s2, ok := val2.(string) would have returned ok==true
		if common.MatchesAsteriskPattern(s2, s1) {
			return true, "", nil
		}
		return false, fmt.Sprintf(
			"string values >%s< and >%s< don't match (case-sensitively "+
				"and allowing asterisk wildcards in second string)", s2, s1), nil

	case isInteger(kind1) && isInteger(kind2):
		if val1 == val2 {
			return true, "", nil
		}
		return false, fmt.Sprintf("integer value %v differs from %v", val1, val2), nil

	case isFloat(kind1) && isFloat(kind2):
		if val1 == val2 {
			return true, "", nil
		}
		return false, fmt.Sprintf("float value %v differs from %v", val1, val2), nil
	}

	return false,
		fmt.Sprintf(
			"type %v (value: %v) differs from type %v (value: %v)",
			kind1.String(), val1, kind2.String(), val2),
		nil // by definition (kind1 and kind2 are different)
}
