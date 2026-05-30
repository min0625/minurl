// Copyright 2024 The MinURL Authors

package testhelpers

import "slices"

// StringSliceContains checks if a string slice contains a specific value.
func StringSliceContains(values []string, want string) bool {
	return slices.Contains(values, want)
}
