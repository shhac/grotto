package form

import (
	"fmt"
	"math"
	"strconv"
)

// ValidateInt32 validates that a string can be parsed as a 32-bit signed integer
func ValidateInt32(s string) error {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid integer: %w", err)
	}
	if val < math.MinInt32 || val > math.MaxInt32 {
		return fmt.Errorf("value %d out of range for int32 (min: %d, max: %d)", val, math.MinInt32, math.MaxInt32)
	}
	return nil
}

// ValidateInt64 validates that a string can be parsed as a 64-bit signed integer
func ValidateInt64(s string) error {
	_, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid integer: %w", err)
	}
	return nil
}

// ValidateUint32 validates that a string can be parsed as a 32-bit unsigned integer
func ValidateUint32(s string) error {
	val, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid unsigned integer: %w", err)
	}
	if val > math.MaxUint32 {
		return fmt.Errorf("value %d out of range for uint32 (max: %d)", val, uint32(math.MaxUint32))
	}
	return nil
}

// ValidateUint64 validates that a string can be parsed as a 64-bit unsigned integer
func ValidateUint64(s string) error {
	_, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid unsigned integer: %w", err)
	}
	return nil
}

// ValidateFloat validates that a string can be parsed as a 32-bit float
func ValidateFloat(s string) error {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("invalid float: %w", err)
	}
	// Check for overflow (float32 range)
	if val != 0 && (val < -math.MaxFloat32 || val > math.MaxFloat32) {
		return fmt.Errorf("value out of range for float32")
	}
	return nil
}

// ValidateDouble validates that a string can be parsed as a 64-bit float
func ValidateDouble(s string) error {
	_, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("invalid double: %w", err)
	}
	return nil
}
