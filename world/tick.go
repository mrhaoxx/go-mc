// This file is part of go-mc/server project.
// Copyright (C) 2023.  Tnze
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package world

import (
	"fmt"
	"math"
	"time"

	"github.com/mrhaoxx/go-mc/chat"
	"github.com/mrhaoxx/go-mc/world/internal/bvh"
	"go.uber.org/zap"
)

func (w *World) tickLoop() {
	var n uint
	for range time.Tick(time.Millisecond * 20) {
		w.tick(n)
		n++
	}
}

var i = 0

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func (w *World) tick(n uint) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()

	// Every 10 ticks, spawn an orbiting pig near each player.
	// if n%100 == 0 {
	// 	for _, p := range w.players {
	// 		angle := rand.Float64() * 2 * math.Pi
	// 		radius := 12.0
	// 		pos := [3]float64{p.Position[0] + radius*math.Cos(angle), p.Position[1], p.Position[2] + radius*math.Sin(angle)}
	// 		w.staticEntities = append(w.staticEntities, simpleEntity{
	// 			Entity:      Entity{EntityID: NewEntityID(), Position: pos, Rotation: [2]float32{0, 0}, OnGround: true, UUID: uuid.New()},
	// 			TypeName:    "minecraft:pig",
	// 			OrbitCenter: p,
	// 			OrbitRadius: radius,
	// 			OrbitSpeed:  0.05, // ~1 rad/sec at 20 TPS
	// 			Angle:       angle,
	// 		})
	// 	}
	// }

	if n%8 == 0 {
		w.subtickChunkLoad()
	}

	// if n%32 == 0 {
	// 	for t, lc := range w.chunks {
	// 		// lc.SetBlock(rand.Intn(16), rand.Intn(16), rand.Intn(16), block.ToStateID[block.Stone{}])]
	// 		lc.Chunk.Sections[10].SetBlock(abs(i+int(t[0])*16)%(16*16*16), block.ToStateID[block.Stone{}])
	// 	}
	// 	i++
	// }

	// if n%128 == 0 {
	// 	for _, lc := range w.chunks {
	// 		lc.Chunk.Sections[10].RotateSection(180, 180)
	// 	}
	// }

	if n%16 == 0 {
		for _, lc := range w.chunks {
			lc.UpdateToViewers()
		}
	}

	w.subtickUpdatePlayers()
	w.subtickUpdateEntities()
}

func (w *World) subtickChunkLoad() {
	for c, p := range w.players {
		x := int32(p.Position[0]) >> 4
		y := int32(p.Position[1]) >> 4
		z := int32(p.Position[2]) >> 4
		if newChunkPos := [3]int32{x, y, z}; newChunkPos != p.ChunkPos {
			p.ChunkPos = newChunkPos
			c.SendSetChunkCacheCenter([2]int32{x, z})
		}
	}
	// because of the random traversal order of w.loaders, every loader has the same opportunity, so it's relatively fair.
LoadChunk:
	for viewer, loader := range w.loaders {
		loader.calcLoadingQueue()
		for _, pos := range loader.loadQueue {
			// fmt.Println("loading chunk", pos)
			if !loader.limiter.Allow() { // We reach the player limit. Skip
				fmt.Println("reach player limit")
				break
			}
			if _, ok := w.chunks[pos]; !ok {
				if !w.loadChunk(pos) {
					fmt.Println("reach global limit")
					break LoadChunk // We reach the global limit. skip
				}
			}
			loader.loaded[pos] = struct{}{}
			lc := w.chunks[pos]
			lc.AddViewer(viewer)
			lc.Lock()
			// fmt.Println("update chunk", pos)
			viewer.ViewChunkLoad(pos, lc.Chunk)
			lc.Unlock()
		}
	}
	for viewer, loader := range w.loaders {
		loader.calcUnusedChunks()
		for _, pos := range loader.unloadQueue {
			delete(loader.loaded, pos)
			if !w.chunks[pos].RemoveViewer(viewer) {
				w.log.Panic("viewer is not found in the loaded chunk")
			}
			viewer.ViewChunkUnload(pos)
		}
	}
	var unloadQueue [][2]int32
	for pos, chunk := range w.chunks {
		if len(chunk.viewers) == 0 {
			unloadQueue = append(unloadQueue, pos)
		}
	}
	for i := range unloadQueue {
		w.unloadChunk(unloadQueue[i])
	}
}

func (w *World) subtickUpdatePlayers() {
	for c, p := range w.players {
		if !p.Inputs.TryLock() {
			continue
		}
		inputs := &p.Inputs
		// update the range of visual.
		// if p.ViewDistance != int32(inputs.ViewDistance) {
		// 	p.ViewDistance = int32(inputs.ViewDistance)
		// 	fmt.Println("updating view distance", p.ViewDistance)
		// 	p.view = w.playerViews.Insert(p.getView(), w.playerViews.Delete(p.view))
		// }
		// delete entities that not in range from entities lists of each player.
		for id, e := range p.EntitiesInView {
			if !p.view.Box.WithIn(vec3d(e.Position)) {
				delete(p.EntitiesInView, id) // it should be safe to delete element from a map being traversed.
				p.view.Value.ViewRemoveEntities([]int32{id})
			}
		}
		if p.teleport != nil {
			if inputs.TeleportID == p.teleport.ID {
				p.pos0 = p.teleport.Position
				p.rot0 = p.teleport.Rotation
				p.teleport = nil
			}
		} else {
			delta := [3]float64{
				inputs.Position[0] - p.Position[0],
				inputs.Position[1] - p.Position[1],
				inputs.Position[2] - p.Position[2],
			}
			distance := math.Sqrt(delta[0]*delta[0] + delta[1]*delta[1] + delta[2]*delta[2])
			if distance > 100 {
				// You moved too quickly :( (Hacking?)
				teleportID := c.SendPlayerPosition(p.Position, p.Rotation)
				p.teleport = &TeleportRequest{
					ID:       teleportID,
					Position: p.Position,
					Rotation: p.Rotation,
				}
			} else if inputs.Position[1] < -100 {

				p.Position[1] = 100

				teleportID := c.SendPlayerPosition(p.Position, p.Rotation)
				p.teleport = &TeleportRequest{
					ID:       teleportID,
					Position: p.Position,
					Rotation: p.Rotation,
				}
			} else if inputs.Position.IsValid() {
				p.pos0 = inputs.Position
				p.rot0 = inputs.Rotation
				p.OnGround = inputs.OnGround
			} else {
				w.log.Info("Player move invalid",
					zap.Float64("x", inputs.Position[0]),
					zap.Float64("y", inputs.Position[1]),
					zap.Float64("z", inputs.Position[2]),
				)
				c.SendDisconnect(chat.TranslateMsg("multiplayer.disconnect.invalid_player_movement"))
			}
		}
		p.Inputs.Unlock()
	}
}

func (w *World) subtickUpdateEntities() {
	// Update and broadcast static/demo entities (including orbiting pigs).
	for i := range w.staticEntities {
		e := &w.staticEntities[i]
		// Determine target position/rotation for this tick.
		if e.OrbitCenter != nil {
			e.Angle += e.OrbitSpeed
			// Orbit horizontally around the player at OrbitRadius.
			nx := e.OrbitCenter.Position[0] + e.OrbitRadius*math.Cos(e.Angle)
			nz := e.OrbitCenter.Position[2] + e.OrbitRadius*math.Sin(e.Angle)
			ny := e.OrbitCenter.Position[1]
			e.pos0 = [3]float64{nx, ny, nz}
			// Face tangentially along the orbit path.
			yaw := float32((e.Angle + math.Pi/2) * 180 / math.Pi)
			e.rot0 = [2]float32{yaw, 0}
			// Tangential velocity (blocks/tick).
			vx := -e.OrbitRadius * e.OrbitSpeed * math.Sin(e.Angle)
			vz := e.OrbitRadius * e.OrbitSpeed * math.Cos(e.Angle)
			e.vel0 = [3]float64{vx, 0, vz}
		} else {
			e.pos0 = e.Position
			e.rot0 = e.Rotation
			e.vel0 = e.Velocity
		}

		// Ensure entity is spawned for viewers in range.
		condForView := bvh.TouchPoint[vec3d, aabb3d](vec3d(e.pos0))
		w.playerViews.Find(condForView, func(n *playerViewNode) bool {
			if _, ok := n.Value.EntitiesInView[e.EntityID]; !ok {
				n.Value.ViewAddEntity(&e.Entity, e.TypeName)
				n.Value.EntitiesInView[e.EntityID] = &e.Entity
				n.Value.ViewSetEntityMotion(e.EntityID, e.vel0)
			}
			return true
		})

		// If entity moved/rotated, send movement updates to viewers.
		var delta [3]int16
		var rot [2]int8
		moved := e.Position != e.pos0
		rotated := e.Rotation != e.rot0
		if moved {
			delta = [3]int16{
				int16((e.pos0[0] - e.Position[0]) * 32 * 128),
				int16((e.pos0[1] - e.Position[1]) * 32 * 128),
				int16((e.pos0[2] - e.Position[2]) * 32 * 128),
			}
		}
		if rotated {
			rot = [2]int8{
				int8(e.rot0[0] * 256 / 360),
				int8(e.rot0[1] * 256 / 360),
			}
		}
		if moved || rotated {
			sendMove := func(v EntityViewer) {}
			switch {
			case moved && rotated:
				sendMove = func(v EntityViewer) {
					v.ViewMoveEntityPosAndRot(e.EntityID, delta, rot, bool(e.OnGround))
					v.ViewRotateHead(e.EntityID, rot[0])
				}
			case moved:
				sendMove = func(v EntityViewer) {
					v.ViewMoveEntityPos(e.EntityID, delta, bool(e.OnGround))
				}
			case rotated:
				sendMove = func(v EntityViewer) {
					v.ViewMoveEntityRot(e.EntityID, rot, bool(e.OnGround))
					v.ViewRotateHead(e.EntityID, rot[0])
				}
			}
			w.playerViews.Find(condForView, func(n *playerViewNode) bool {
				if _, ok := n.Value.EntitiesInView[e.EntityID]; ok {
					sendMove(n.Value.EntityViewer)
				} else {
					// Not visible yet — spawn now and mark.
					n.Value.ViewAddEntity(&e.Entity, e.TypeName)
					n.Value.EntitiesInView[e.EntityID] = &e.Entity
					n.Value.ViewSetEntityMotion(e.EntityID, e.vel0)
				}
				return true
			})
			// Commit new pose and update velocity if changed.
			e.Position = e.pos0
			e.Rotation = e.rot0
			const eps = 1e-3
			if math.Abs(e.vel0[0]-e.Velocity[0]) > eps || math.Abs(e.vel0[1]-e.Velocity[1]) > eps || math.Abs(e.vel0[2]-e.Velocity[2]) > eps {
				w.playerViews.Find(condForView, func(n *playerViewNode) bool {
					if _, ok := n.Value.EntitiesInView[e.EntityID]; ok {
						n.Value.ViewSetEntityMotion(e.EntityID, e.vel0)
					}
					return true
				})
				e.Velocity = e.vel0
			}
		}
	}

	// Players are also entities; update movement and ensure visibility to others.
	// TODO: entity list should be traversed here, but players are the only entities now.
	for _, e := range w.players {
		// sending Update Entity Position pack to every player who can see it, when it moves.
		var delta [3]int16
		var rot [2]int8
		if e.Position != e.pos0 { // TODO: send Teleport Entity pack instead when moving distance is greater than 8.
			delta = [3]int16{
				int16((e.pos0[0] - e.Position[0]) * 32 * 128),
				int16((e.pos0[1] - e.Position[1]) * 32 * 128),
				int16((e.pos0[2] - e.Position[2]) * 32 * 128),
			}
		}
		if e.Rotation != e.rot0 {
			rot = [2]int8{
				int8(e.rot0[0] * 256 / 360),
				int8(e.rot0[1] * 256 / 360),
			}
		}
		cond := bvh.TouchPoint[vec3d, aabb3d](vec3d(e.Position))
		w.playerViews.Find(cond,
			func(n *playerViewNode) bool {
				if n.Value.Player == e {
					return true // don't send the player self to the player
				}
				// check if the current entity is in range of player visual. if so, moving data will be forwarded.
				if _, ok := n.Value.EntitiesInView[e.EntityID]; !ok {
					// add the entity to the entity list of the player
					n.Value.ViewAddPlayer(e)
					n.Value.EntitiesInView[e.EntityID] = &e.Entity
				}
				return true
			},
		)
		var sendMove func(v EntityViewer)
		switch {
		case e.Position != e.pos0 && e.Rotation != e.rot0:
			sendMove = func(v EntityViewer) {
				v.ViewMoveEntityPosAndRot(e.EntityID, delta, rot, bool(e.OnGround))
				v.ViewRotateHead(e.EntityID, rot[0])
			}
		case e.Position != e.pos0:
			sendMove = func(v EntityViewer) {
				v.ViewMoveEntityPos(e.EntityID, delta, bool(e.OnGround))
			}
		case e.Rotation != e.rot0:
			sendMove = func(v EntityViewer) {
				v.ViewMoveEntityRot(e.EntityID, rot, bool(e.OnGround))
				v.ViewRotateHead(e.EntityID, rot[0])
			}
		default:
			continue
		}
		e.Position = e.pos0
		e.Rotation = e.rot0
		w.playerViews.Find(cond,
			func(n *playerViewNode) bool {
				if n.Value.Player == e {
					return true // not sending self movements to player self.
				}
				// check if the current entity is in the player visual entities list. if so, moving data will be forwarded.
				if _, ok := n.Value.EntitiesInView[e.EntityID]; ok {
					sendMove(n.Value.EntityViewer)
				} else {
					// or the entity will be add to the entities list of the player
					// TODO: deal with the situation that the entity is not a player
					n.Value.ViewAddPlayer(e)
					n.Value.EntitiesInView[e.EntityID] = &e.Entity
				}
				return true
			},
		)
	}
}
