// Copyright 2017-2018 DigitalOcean.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command lsdimms lists memory DIMM information from SMBIOS.
package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/digitalocean/go-smbios/smbios"
)

const (
	// typeMemoryDevice is the SMBIOS Memory Device (Type 17) structure type.
	typeMemoryDevice = 17

	// Offsets into a Memory Device structure's formatted section.  The
	// offsets in the specification include the 4 byte structure header,
	// which is not part of Structure.Formatted.
	offsetSize          = 0x0c - 4
	offsetDeviceLocator = 0x10 - 4
	offsetAttributes    = 0x1b - 4
	offsetExtendedSize  = 0x1c - 4

	// sizeUnknown indicates that the Size field is unknown.
	sizeUnknown = 0xffff

	// sizeExtended indicates that the actual size is stored in the Extended
	// Size field.
	sizeExtended = 0x7fff

	// sizeKilobytes is set in the Size field when the value is specified in
	// kilobytes, rather than megabytes.
	sizeKilobytes = 0x8000

	// Memory Device Attributes bits, added in SMBIOS 3.10.0.
	attrUnmapped = 1 << 6
	attrDisabled = 1 << 5
)

func main() {
	// Find SMBIOS data in operating system-specific location.
	rc, ep, err := smbios.Stream()
	if err != nil {
		log.Fatalf("failed to open stream: %v", err)
	}
	// Be sure to close the stream!
	defer rc.Close()

	// Decode SMBIOS structures from the stream.
	d := smbios.NewDecoder(rc)
	ss, err := d.Decode()
	if err != nil {
		log.Fatalf("failed to decode structures: %v", err)
	}

	major, minor, rev := ep.Version()
	fmt.Printf("SMBIOS %d.%d.%d\n", major, minor, rev)

	for _, s := range ss {
		// Only look at memory devices.
		if s.Header.Type != typeMemoryDevice {
			continue
		}

		// Code based on: https://www.dmtf.org/sites/default/files/standards/documents/DSP0134_3.10.0.pdf.

		// TODO: this should go in a new package in go-smbios for parsing specific structures.

		// Structures from older versions of the specification may be too
		// short to contain the fields parsed below.
		if len(s.Formatted) < offsetSize+2 {
			continue
		}

		locator := stringAt(s, offsetDeviceLocator)

		// Only parse the DIMM size and state.
		size := binary.LittleEndian.Uint16(s.Formatted[offsetSize:])

		var state string
		if len(s.Formatted) > offsetAttributes {
			attrs := s.Formatted[offsetAttributes]
			if attrs&attrUnmapped != 0 {
				state += " (unmapped)"
			}
			if attrs&attrDisabled != 0 {
				state += " (disabled)"
			}
		}

		switch size {
		case 0:
			fmt.Printf("[% 3s] empty%s\n", locator, state)
			continue
		case sizeUnknown:
			fmt.Printf("[% 3s] DIMM: unknown%s\n", locator, state)
			continue
		}

		var amount uint32
		unit := "MB"

		switch {
		case size == sizeExtended:
			// The Extended Size field is only present in structures from
			// SMBIOS 2.7 and later.  Bit 31 is reserved, and the value is
			// always specified in megabytes.
			if len(s.Formatted) < offsetExtendedSize+4 {
				fmt.Printf("[% 3s] DIMM: unknown%s\n", locator, state)
				continue
			}
			amount = binary.LittleEndian.Uint32(s.Formatted[offsetExtendedSize:]) & 0x7fffffff
		case size&sizeKilobytes != 0:
			// The granularity in which the value is specified depends on
			// the setting of the most-significant bit (bit 15). If the bit
			// is 0, the value is specified in megabyte units; if the bit is
			// 1, the value is specified in kilobyte units.
			amount = uint32(size &^ sizeKilobytes)
			unit = "KB"
		default:
			amount = uint32(size)
		}

		fmt.Printf("[% 3s] DIMM: %d %s%s\n", locator, amount, unit, state)
	}
}

// stringAt returns the string referenced by the string number stored at the
// given offset of s's formatted section, or an empty string if it is not
// present.  String numbers are 1-indexed, and 0 means no string.
func stringAt(s *smbios.Structure, offset int) string {
	if offset >= len(s.Formatted) {
		return ""
	}

	n := int(s.Formatted[offset])
	if n == 0 || n > len(s.Strings) {
		return ""
	}

	return s.Strings[n-1]
}
