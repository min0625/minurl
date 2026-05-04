// Copyright 2024 The MinURL Authors

package testhelpers

// StringSliceContains checks if a string slice contains a specific value.
func StringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
