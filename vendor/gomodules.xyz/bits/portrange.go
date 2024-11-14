package bits

import (
	"errors"
	"fmt"
	"strings"
)

// PortRange struct for managing a range of ports with a starting index and fixed size
type PortRange struct {
	startPort int       // Starting port of the range
	size      int       // Number of ports in the range
	bitField  *BitField // BitField to track port usage within the range
}

// NewPortRange creates a new PortRange with a specified startPort and size
func NewPortRange(startPort, size int) (*PortRange, error) {
	if startPort < 0 || size <= 0 {
		return nil, errors.New("invalid start port or size")
	}

	return &PortRange{
		startPort: startPort,
		size:      size,
		bitField:  NewBitField(size),
	}, nil
}

// AllocateNextPorts allocates the next `n` available ports in the range.
// It uses `SetNextAvailableBits` to find non-consecutive available ports.
func (pr *PortRange) AllocateNextPorts(n int) ([]int, error) {
	if n <= 0 || n > pr.size {
		return nil, errors.New("invalid number of ports to allocate")
	}

	// Find and allocate `n` available bits
	allocatedBits, err := pr.bitField.SetNextAvailableBits(n)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate ports: %w", err)
	}

	// Map the bit positions to actual port numbers
	allocatedPorts := make([]int, len(allocatedBits))
	for i, bitPos := range allocatedBits {
		allocatedPorts[i] = pr.startPort + bitPos
	}

	return allocatedPorts, nil
}

// ReleasePorts releases a specified list of ports in the range
func (pr *PortRange) ReleasePorts(ports []int) error {
	for _, port := range ports {
		if port < pr.startPort || port >= pr.startPort+pr.size {
			return errors.New("port out of range")
		}
		pos := port - pr.startPort
		pr.bitField.ClearBit(pos)
	}
	return nil
}

// IsPortAllocated checks if a specific port in the range is allocated
func (pr *PortRange) IsPortAllocated(port int) (bool, error) {
	if port < pr.startPort || port >= pr.startPort+pr.size {
		return false, errors.New("port out of range")
	}
	return pr.bitField.IsSet(port - pr.startPort), nil
}

// SetPortAllocated sets a specific port in the range as allocated
func (pr *PortRange) SetPortAllocated(port int) error {
	if port < pr.startPort || port >= pr.startPort+pr.size {
		return errors.New("port out of range")
	}

	pos := port - pr.startPort
	pr.bitField.SetBit(pos)
	return nil
}

// String returns a string representation of the allocated fields in the PortRange
func (pr *PortRange) String() string {
	var sb strings.Builder
	sb.WriteString("Allocated ports: [")
	for i := 0; i < pr.size; i++ {
		if pr.bitField.IsSet(i) {
			port := pr.startPort + i
			sb.WriteString(fmt.Sprintf("%d ", port))
		}
	}
	sb.WriteString("]")
	return sb.String()
}

/*
func main_() {
	// Create a PortRange from port 1000 with a range of 20 ports
	portRange, err := NewPortRange(1000, 20)
	if err != nil {
		fmt.Println("Error creating port range:", err)
		return
	}

	// Allocate specific port 1003
	err = portRange.SetPortAllocated(1003)
	if err != nil {
		fmt.Println("Error allocating port:", err)
	} else {
		fmt.Println("Port 1003 allocated")
	}

	// Check if port 1003 is allocated
	isAllocated, _ := portRange.IsPortAllocated(1003)
	fmt.Printf("Port 1003 is allocated: %v\n", isAllocated)

	// Try allocating a port out of range
	err = portRange.SetPortAllocated(1025)
	if err != nil {
		fmt.Println("Error:", err)
	}
}

func main__() {
	portRange, _ := NewPortRange(1000, 10)

	// Allocate specific ports
	portRange.SetPortAllocated(1000)
	portRange.SetPortAllocated(1002)
	portRange.SetPortAllocated(1005)

	// Print allocated fields
	fmt.Println(portRange.String()) // Expected: Allocated ports: [1000 1002 1005 ]
}
*/
