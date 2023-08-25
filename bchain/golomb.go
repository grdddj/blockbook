package bchain

import (
	"bytes"
	"encoding/hex"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/martinboehm/btcutil/gcs"
)

type FilterScriptsType int

const (
	FilterScriptsInvalid = FilterScriptsType(iota)
	FilterScriptsAll
	FilterScriptsTaproot
	FilterScriptsTaprootNoOrdinals
)

// GolombFilter is computing golomb filter of address descriptors
type GolombFilter struct {
	Enabled           bool
	p                 uint8
	key               string
	filterScripts     string
	filterScriptsType FilterScriptsType
	filterData        [][]byte
	uniqueData        map[string]struct{}
}

// NewGolombFilter initializes the GolombFilter handler
func NewGolombFilter(p uint8, filterScripts string, key string) (*GolombFilter, error) {
	if p == 0 {
		return &GolombFilter{Enabled: false}, nil
	}
	gf := GolombFilter{
		Enabled:           true,
		p:                 p,
		key:               key,
		filterScripts:     filterScripts,
		filterScriptsType: filterScriptsToScriptsType(filterScripts),
		filterData:        make([][]byte, 0),
		uniqueData:        make(map[string]struct{}),
	}
	// only taproot and all is supported
	if gf.filterScriptsType == FilterScriptsInvalid {
		return nil, errors.Errorf("Invalid/unsupported filterScripts parameter %s", filterScripts)
	}
	return &gf, nil
}

// Checks whether this input contains ordinal data
func isInputOrdinal(vin Vin) bool {
	byte_pattern := []byte{
		0x00, // OP_0, OP_FALSE
		0x63, // OP_IF
		0x03, // OP_PUSHBYTES_3
		0x6f, // "o"
		0x72, // "r"
		0x64, // "d"
		0x01, // OP_PUSHBYTES_1
	}
	// Witness needs to have at least 3 items and the second one needs to contain certain pattern
	return len(vin.Witness) > 2 && bytes.Contains(vin.Witness[1], byte_pattern)
}

func txContainsOrdinal(tx *Tx) bool {
	for _, vin := range tx.Vin {
		if isInputOrdinal(vin) {
			return true
		}
	}
	return false
}

// AddAddrDesc adds taproot address descriptor to the data for the filter
func (f *GolombFilter) AddAddrDesc(ad AddressDescriptor, tx *Tx) {
	if f.ignoreNonTaproot() && !ad.IsTaproot() {
		return
	}
	if f.ignoreOrdinals() && tx != nil && txContainsOrdinal(tx) {
		return
	}
	if len(ad) == 0 {
		return
	}
	s := string(ad)
	if _, found := f.uniqueData[s]; !found {
		f.filterData = append(f.filterData, ad)
		f.uniqueData[s] = struct{}{}
	}
}

// Compute computes golomb filter from the data
func (f *GolombFilter) Compute() []byte {
	m := uint64(1 << uint64(f.p))

	if len(f.filterData) == 0 {
		return nil
	}

	b, _ := hex.DecodeString(f.key)
	if len(b) < gcs.KeySize {
		return nil
	}

	filter, err := gcs.BuildGCSFilter(f.p, m, *(*[gcs.KeySize]byte)(b[:gcs.KeySize]), f.filterData)
	if err != nil {
		glog.Error("Cannot create golomb filter for ", f.key, ", ", err)
		return nil
	}

	fb, err := filter.NBytes()
	if err != nil {
		glog.Error("Error getting NBytes from golomb filter for ", f.key, ", ", err)
		return nil
	}

	return fb
}

func (f *GolombFilter) ignoreNonTaproot() bool {
	switch f.filterScriptsType {
	case FilterScriptsTaproot, FilterScriptsTaprootNoOrdinals:
		return true
	}
	return false
}

func (f *GolombFilter) ignoreOrdinals() bool {
	switch f.filterScriptsType {
	case FilterScriptsTaprootNoOrdinals:
		return true
	}
	return false
}

func filterScriptsToScriptsType(filterScripts string) FilterScriptsType {
	switch filterScripts {
	case "":
		return FilterScriptsAll
	case "taproot":
		return FilterScriptsTaproot
	case "taproot-noordinals":
		return FilterScriptsTaprootNoOrdinals
	}
	return FilterScriptsInvalid
}
