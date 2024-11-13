package bits

import (
	"errors"
	"strings"
)

// BitField struct holds a slice of uint64 integers for 64-bit blocks
type BitField struct {
	data []uint64
	size int // Number of bits the BitField can hold
}

// NewBitField creates a BitField capable of holding 'n' bits
func NewBitField(n int) *BitField {
	numInts := (n + 63) / 64 // Calculate number of uint64 required
	return &BitField{
		data: make([]uint64, numInts),
		size: n,
	}
}

// SetBit sets the bit at position 'pos' to 1
func (bf *BitField) SetBit(pos int) {
	index, bit := pos/64, uint(pos%64)
	bf.data[index] |= (1 << bit)
}

// ClearBit clears the bit at position 'pos' (sets it to 0)
func (bf *BitField) ClearBit(pos int) {
	index, bit := pos/64, uint(pos%64)
	bf.data[index] &^= (1 << bit)
}

// IsSet checks if the bit at position 'pos' is set (1)
func (bf *BitField) IsSet(pos int) bool {
	index, bit := pos/64, uint(pos%64)
	return (bf.data[index] & (1 << bit)) != 0
}

// SetNextAvailableBits finds and sets the next `n` available bits, returning their positions.
// It does not require the bits to be consecutive.
func (bf *BitField) SetNextAvailableBits(n int) ([]int, error) {
	if n <= 0 || n > bf.size {
		return nil, errors.New("invalid number of bits to allocate")
	}

	allocated := make([]int, 0, n)
	for pos := 0; pos < bf.size && len(allocated) < n; pos++ {
		if !bf.IsSet(pos) {
			bf.SetBit(pos)
			allocated = append(allocated, pos)
		}
	}

	if len(allocated) < n {
		// Not enough available bits found
		return nil, errors.New("not enough available bits")
	}

	return allocated, nil
}

// NextAvailableBitsInRange finds the next `n` available (unset) bits within a specified range [start, end).
// It does not require the bits to be consecutive.
func (bf *BitField) NextAvailableBitsInRange(start, end, n int) ([]int, error) {
	// Validate input range
	if start < 0 || end > bf.size || start >= end || n <= 0 {
		return nil, errors.New("invalid input parameters")
	}

	availableBits := make([]int, 0, n)
	for pos := start; pos < end && len(availableBits) < n; pos++ {
		if !bf.IsSet(pos) {
			availableBits = append(availableBits, pos)
		}
	}

	if len(availableBits) < n {
		// Not enough available bits found
		return nil, errors.New("not enough available bits in range")
	}

	return availableBits, nil
}

// String returns a string representation of the BitField.
func (bf *BitField) String() string {
	var sb strings.Builder
	sb.WriteString("BitField: [")
	for i := 0; i < bf.size; i++ {
		if bf.IsSet(i) {
			sb.WriteString("1")
		} else {
			sb.WriteString("0")
		}
	}
	sb.WriteString("]")
	return sb.String()
}

/*
func main() {
	bitField := NewBitField(128) // Initialize bit field with 128 bits

	// Set next available 5 bits
	startPos, err := bitField.SetNextAvailableBits(5)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Next available 5 bits set starting at position %d\n", startPos)
	}

	// Check if the bits are set
	for i := startPos; i < startPos+5; i++ {
		fmt.Printf("Bit %d is set: %v\n", i, bitField.IsSet(i))
	}
}

func main_1() {
	// Create a BitField of size 10
	bitField := NewBitField(10)

	// Set bits at specific positions
	bitField.SetBit(1)
	bitField.SetBit(3)
	bitField.SetBit(5)

	// Print BitField representation
	fmt.Println(bitField.String()) // Expected: BitField: [0101010000]
}
*/
