package form

import (
	"fmt"
	"math"
	"strconv"

	"google.golang.org/protobuf/reflect/protoreflect"
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

// parseScalarValue parses a string into the appropriate scalar type based on field descriptor
func parseScalarValue(s string, fd protoreflect.FieldDescriptor) (interface{}, error) {
	switch fd.Kind() {
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		val, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid int32: %w", err)
		}
		if val < math.MinInt32 || val > math.MaxInt32 {
			return nil, fmt.Errorf("value out of range for int32")
		}
		return int32(val), nil

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		val, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid int64: %w", err)
		}
		return val, nil

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		val, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid uint32: %w", err)
		}
		if val > math.MaxUint32 {
			return nil, fmt.Errorf("value out of range for uint32")
		}
		return uint32(val), nil

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		val, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid uint64: %w", err)
		}
		return val, nil

	case protoreflect.FloatKind:
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float: %w", err)
		}
		if val != 0 && (val < -math.MaxFloat32 || val > math.MaxFloat32) {
			return nil, fmt.Errorf("value out of range for float32")
		}
		return float32(val), nil

	case protoreflect.DoubleKind:
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid double: %w", err)
		}
		return val, nil

	case protoreflect.StringKind:
		return s, nil

	case protoreflect.BoolKind:
		val, err := strconv.ParseBool(s)
		if err != nil {
			return nil, fmt.Errorf("invalid bool: %w", err)
		}
		return val, nil

	default:
		return nil, fmt.Errorf("unsupported scalar type: %v", fd.Kind())
	}
}
