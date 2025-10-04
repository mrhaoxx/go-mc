package client

import (
	"github.com/mrhaoxx/go-mc/level/component"
	pk "github.com/mrhaoxx/go-mc/net/packet"
)

type ItemStack struct {
	ItemID     int32
	Count      byte
	Components []component.DataComponent
}

func (s *ItemStack) isEmpty() bool {
	if s == nil {
		return true
	}
	if s.ItemID == 0 {
		return true
	}
	if s.Count == 0 {
		return true
	}
	return false
}

func (s *ItemStack) encodeFields() []pk.FieldEncoder {
	if s == nil || s.isEmpty() {
		return []pk.FieldEncoder{pk.VarInt(0)}
	}

	fields := []pk.FieldEncoder{
		pk.VarInt(s.Count), // non-empty
		pk.VarInt(s.ItemID),
		pk.VarInt(0),
		pk.VarInt(0), // No NBT
	}

	// fields = append(fields, encodeComponentPatch(s.Components)...)
	return fields
}
