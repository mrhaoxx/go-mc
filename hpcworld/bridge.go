package hpcworld

/*
#include "bridge.h"
*/
import "C"

type Bridge struct {
	handle uintptr

	ViewMoveEntityPos       func(h uintptr, id int32, delta [3]int16, onGround bool)
	ViewMoveEntityPosAndRot func(h uintptr, id int32, delta [3]int16, rot [2]int8, onGround bool)
	ViewRemoveEntities      func(h uintptr, ids []int32)

	SendDisconnectJSON         func(h uintptr, js string)
	SendSetPlayerInventorySlot func(h uintptr, slot int32, stackHandle uintptr)
	SendSetPlayerMainHandSlot  func(h uintptr, stackHandle uintptr)
	SendSetPlayerOffHandSlot   func(h uintptr, stackHandle uintptr)
	SendSetPlayerArmorSlot     func(h uintptr, slot int32, stackHandle uintptr)
	SendSetPlayerCursorItem    func(h uintptr, stackHandle uintptr)
	SendOpenPlayerInventory    func(h uintptr)
}

func init() {

}
