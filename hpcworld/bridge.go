package hpcworld

/*
// #cgo LDFLAGS: -Wl,--unresolved-symbols=ignore-all
#cgo LDFLAGS: -Wl,-undefined,dynamic_lookup
#include "bridge.h"
// Expose load_chunk prototype for cgo
Chunk* load_chunk(int32_t x, int32_t z);
*/
import "C"
import (
	"unsafe"

	"github.com/mrhaoxx/go-mc/level"
	"github.com/mrhaoxx/go-mc/level/block"
)

// cgo: expose load_chunk
/*
Chunk* load_chunk(int32_t x, int32_t z);
*/

// LoadChunk wraps the C load_chunk function and returns a Go pointer to Chunk
func LoadChunk(x, z int32) *Chunk {
	cChunk := C.load_chunk(C.int32_t(x), C.int32_t(z))
	if cChunk == nil {
		return nil
	}
	return ToGoChunk(cChunk)
}

type Section struct {
	Blockcount int16

	BlocksState [4096]int32
	Biomes      [64]int32
	SkyLight    [2048]int8
	BlockLight  [2048]int8
}

// SetBlock sets the block at index i to v, updating Blockcount like level.Section.SetBlock
func (s *Section) SetBlock(i int, v int32) {
	// Air block is 0 in go-mc/level/block
	if s.BlocksState[i] != 0 {
		s.Blockcount--
	}
	if v != 0 {
		s.Blockcount++
	}
	s.BlocksState[i] = v
}

type Chunk struct {
	Sections [24]Section
}

func init() {
	var s Section
	if unsafe.Sizeof(s) != uintptr(C.Section_size) {
		panic("Section size mismatch")
	}
	if unsafe.Alignof(s) != uintptr(C.Section_align) {
		panic("Section align mismatch")
	}
	if unsafe.Offsetof(s.Blockcount) != uintptr(C.Section_off_blockcount) {
		panic("Section.blockcount off")
	}
	if unsafe.Offsetof(s.BlocksState) != uintptr(C.Section_off_blocks_state) {
		panic("Section.blocks_state off")
	}
	if unsafe.Offsetof(s.Biomes) != uintptr(C.Section_off_biomes) {
		panic("Section.biomes off")
	}
	if unsafe.Offsetof(s.SkyLight) != uintptr(C.Section_off_sky_light) {
		panic("Section.sky_light off")
	}
	if unsafe.Offsetof(s.BlockLight) != uintptr(C.Section_off_block_light) {
		panic("Section.block_light off")
	}

	var c Chunk
	if unsafe.Sizeof(c) != uintptr(C.Chunk_size) {
		panic("Chunk size mismatch")
	}
	if unsafe.Alignof(c) != uintptr(C.Chunk_align) {
		panic("Chunk align mismatch")
	}
	if unsafe.Offsetof(c.Sections) != uintptr(C.Chunk_off_sections) {
		panic("Chunk.sections off")
	}
}

func NewChunk() *C.Chunk                      { return (*C.Chunk)(C.malloc(C.Chunk_size)) }
func FreeChunk(ch *C.Chunk)                   { C.free(unsafe.Pointer(ch)) }
func SectionAt(ch *C.Chunk, i int) *C.Section { return &ch.sections[i] }

// ToGoChunk converts a C Chunk pointer to a Go Chunk pointer for testing
func ToGoChunk(ch *C.Chunk) *Chunk {
	return (*Chunk)(unsafe.Pointer(ch))
}

// ToCChunk converts a Go Chunk pointer back to a C Chunk pointer
func ToCChunk(ch *Chunk) *C.Chunk {
	return (*C.Chunk)(unsafe.Pointer(ch))
}

// ToCSection converts a C Section pointer to a Go Section pointer for testing
func ToCSection(s *C.Section) *Section {
	return (*Section)(unsafe.Pointer(s))
}

// LevelChunkFromHPC converts an HPC C-backed Chunk into a high-level level.Chunk.
// It builds palette containers for block states and biomes and copies light data.
func LevelChunkFromHPC(ch *Chunk) *level.Chunk {
	// Convert C.Chunk to Go struct view
	goCh := ch
	// HPC has a fixed-size array [24]Section; treat all as present
	secs := len(goCh.Sections)
	lc := level.EmptyChunk(secs)

	// Populate sections
	for si := 0; si < secs; si++ {
		hpcSec := &goCh.Sections[si]
		lvlSec := &lc.Sections[si]

		// States: create an empty palette container then set every block index
		lvlSec.States = level.NewStatesPaletteContainer(16*16*16, 0)
		for i := 0; i < 16*16*16; i++ {
			// BlocksState in HPC is int32; level.BlocksState is alias of int
			lvlSec.SetBlock(i, level.BlocksState(hpcSec.BlocksState[i]))
		}

		// Biomes: 4*4*4 = 64 entries
		lvlSec.Biomes = level.NewBiomesPaletteContainer(4*4*4, 0)
		for i := 0; i < 4*4*4; i++ {
			lvlSec.Biomes.Set(i, level.BiomesState(hpcSec.Biomes[i]))
		}

		// Lights: copy raw nibble-packed arrays (2048 bytes)
		// SkyLight
		if lvlSec.SkyLight == nil || len(lvlSec.SkyLight) != 2048 {
			lvlSec.SkyLight = make([]byte, 2048)
		}
		for i := 0; i < 2048; i++ {
			lvlSec.SkyLight[i] = byte(hpcSec.SkyLight[i])
		}
		// BlockLight
		if lvlSec.BlockLight == nil || len(lvlSec.BlockLight) != 2048 {
			lvlSec.BlockLight = make([]byte, 2048)
		}
		for i := 0; i < 2048; i++ {
			lvlSec.BlockLight[i] = byte(hpcSec.BlockLight[i])
		}
	}

	// Status: no HPC notion; default to empty
	lc.Status = level.StatusEmpty

	return lc
}

// HPCChunkFromLevel converts a level.Chunk into an HPC C-backed Chunk.
// It allocates a new C.Chunk and fills sections with states, biomes, and light data.
func HPCChunkFromLevel(lc *level.Chunk) *C.Chunk {
	// Allocate C chunk
	cch := NewChunk()
	goCh := ToGoChunk(cch)

	// Determine how many sections to fill (cap at HPC's fixed 24)
	secs := len(lc.Sections)
	if secs > len(goCh.Sections) {
		secs = len(goCh.Sections)
	}

	for si := 0; si < secs; si++ {
		lvlSec := &lc.Sections[si]
		hpcSec := &goCh.Sections[si]

		// Blocks: 4096 entries
		var blockCount int16
		for i := 0; i < 16*16*16; i++ {
			v := int32(lvlSec.GetBlock(i))
			hpcSec.BlocksState[i] = v
			// Count non-air blocks using canonical helper
			if !block.IsAir(level.BlocksState(v)) {
				blockCount++
			}
		}
		hpcSec.Blockcount = blockCount

		// Biomes: 64 entries
		for i := 0; i < 4*4*4; i++ {
			hpcSec.Biomes[i] = int32(lvlSec.Biomes.Get(i))
		}

		// Lights: copy nibble-packed arrays
		// SkyLight
		for i := 0; i < 2048; i++ {
			var b byte
			if lvlSec.SkyLight != nil && i < len(lvlSec.SkyLight) {
				b = lvlSec.SkyLight[i]
			}
			hpcSec.SkyLight[i] = int8(b)
		}
		// BlockLight
		for i := 0; i < 2048; i++ {
			var b byte
			if lvlSec.BlockLight != nil && i < len(lvlSec.BlockLight) {
				b = lvlSec.BlockLight[i]
			}
			hpcSec.BlockLight[i] = int8(b)
		}
	}

	// Zero-out remaining sections if any
	for si := secs; si < len(goCh.Sections); si++ {
		hpcSec := &goCh.Sections[si]
		hpcSec.Blockcount = 0
		// zero arrays are already zeroed in new malloc'd memory, but ensure lights
		// remain zero; nothing else to do.
	}

	return cch
}
