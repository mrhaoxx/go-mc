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
	"sync"

	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/google/uuid"
	"github.com/mrhaoxx/go-mc/hpcworld"
	"github.com/mrhaoxx/go-mc/level"
	"github.com/mrhaoxx/go-mc/world/internal/bvh"
)

type World struct {
	log    *zap.Logger
	config Config
	// chunkProvider ChunkProvider

	chunks    map[[2]int32]*LoadedChunk
	loaders   map[ChunkViewer]*loader
	tickLock  sync.Mutex
	tickCount uint

	// playerViews is a BVH tree，storing the visual range collision boxes of each player.
	// the data structure is used to determine quickly which players to send notify when entity moves.
	playerViews playerViewTree
	players     map[Client]*Player

	// staticEntities are simple demo entities broadcast to clients for visibility testing.
	staticEntities []simpleEntity
}

type Config struct {
	ViewDistance  int32
	SpawnAngle    float32
	SpawnPosition [3]int32
}

type playerView struct {
	EntityViewer
	*Player
}

type (
	vec3d          = bvh.Vec3[float64]
	aabb3d         = bvh.AABB[float64, vec3d]
	playerViewNode = bvh.Node[float64, aabb3d, playerView]
	playerViewTree = bvh.Tree[float64, aabb3d, playerView]
)

func New(logger *zap.Logger, config Config) (w *World) {
	w = &World{
		log:     logger,
		config:  config,
		chunks:  make(map[[2]int32]*LoadedChunk),
		loaders: make(map[ChunkViewer]*loader),
		players: make(map[Client]*Player),
		// chunkProvider: provider,
	}
	// Add a few sample entities near spawn for testing visibility in clients.
	base := [3]float64{float64(config.SpawnPosition[0]) + 2, float64(config.SpawnPosition[1]) + 1, float64(config.SpawnPosition[2]) + 2}
	zap.S().Infof("default base %f %f %f", base[0], base[1], base[2])
	w.staticEntities = []simpleEntity{
		{Entity: Entity{EntityID: NewEntityID(), Position: base, Rotation: [2]float32{0, 0}, OnGround: true, UUID: uuid.New()}, TypeName: "minecraft:armor_stand"},
		{Entity: Entity{EntityID: NewEntityID(), Position: [3]float64{base[0] + 2, base[1], base[2]}, Rotation: [2]float32{0, 0}, OnGround: true, UUID: uuid.New()}, TypeName: "minecraft:pig"},
	}
	go w.tickLoop()
	return
}

type simpleEntity struct {
	Entity
	TypeName    string
	OrbitCenter *Player
	OrbitRadius float64
	OrbitSpeed  float64 // radians per tick
	Angle       float64 // current angle
	Velocity    [3]float64
	vel0        [3]float64
}

func (w *World) Name() string {
	return "minecraft:overworld"
}

func (w *World) SpawnPositionAndAngle() ([3]int32, float32) {
	return w.config.SpawnPosition, w.config.SpawnAngle
}

func (w *World) HashedSeed() [8]byte {
	return [8]byte{}
}

func (w *World) AddPlayer(c Client, p *Player, limiter *rate.Limiter) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	w.loaders[c] = newLoader(p, limiter)
	w.players[c] = p
	p.view = w.playerViews.Insert(p.getView(), playerView{c, p})
}

func (w *World) RemovePlayer(c Client, p *Player) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	w.log.Debug("Remove Player",
		zap.Int("loader count", len(w.loaders[c].loaded)),
		zap.Int("world count", len(w.chunks)),
	)
	// delete the player from all chunks which load the player.
	for pos := range w.loaders[c].loaded {
		if !w.chunks[pos].RemoveViewer(c) {
			w.log.Panic("viewer is not found in the loaded chunk")
		}
	}
	delete(w.loaders, c)
	delete(w.players, c)
	// delete the player from entity system.
	w.playerViews.Delete(p.view)
	w.playerViews.Find(
		bvh.TouchPoint[vec3d, aabb3d](bvh.Vec3[float64](p.Position)),
		func(n *playerViewNode) bool {
			n.Value.ViewRemoveEntities([]int32{p.EntityID})
			delete(n.Value.EntitiesInView, p.EntityID)
			return true
		},
	)
}

func (w *World) loadChunk(pos [2]int32) bool {
	logger := w.log.With(zap.Int32("x", pos[0]), zap.Int32("z", pos[1]))
	logger.Debug("Loading chunk")
	// c, err := w.chunkProvider.GetChunk(pos)
	c := hpcworld.LoadChunk(pos[0], pos[1])

	// if err != nil {
	// 	if errors.Is(err, errChunkNotExist) {
	// 		logger.Debug("Generate chunk")
	// 		// TODO: because there is no chunk generator，generate an empty chunk and mark it as generated
	// 		c = level.EmptyChunk(24)

	// 		var t level.BiomesState
	// 		t.UnmarshalText([]byte("minecraft:plains"))

	// 		for s := range c.Sections {
	// 			for b := 0; b < 4*4*4; b++ {
	// 				c.Sections[s].Biomes.Set(b, t)
	// 			}
	// 			for i := 0; i < 16*16*16; i++ {
	// 				rnd := rand.Intn(16)

	// 				c.Sections[s].SetSkyLight(i, rnd)
	// 				c.Sections[s].SetBlockLight(i, rnd)
	// 			}

	// 			if s != 10 {
	// 				continue
	// 			}

	// 			for i := 0; i < 16*16; i++ {
	// 				// if i == 10 {
	// 				stone := block.GrassBlock{}

	// 				c.Sections[s].SetBlock(i, block.ToStateID[stone])
	// 				c.Sections[s].SetSkyLight(i, i%16)
	// 				c.Sections[s].SetBlockLight(i, i%16)
	// 				// }
	// 			}
	// 		}
	// 		c.Status = level.StatusFull
	// 	} else if !errors.Is(err, ErrReachRateLimit) {
	// 		logger.Error("GetChunk error", zap.Error(err))
	// 		return false
	// 	}
	// }
	w.chunks[pos] = &LoadedChunk{Chunk: c, Pos: level.ChunkPos{pos[0], pos[1]}}
	return true
}

// func (w *World) unloadChunk(pos [2]int32) {
// 	logger := w.log.With(zap.Int32("x", pos[0]), zap.Int32("z", pos[1]))
// 	logger.Debug("Unloading chunk")
// 	c, ok := w.chunks[pos]
// 	if !ok {
// 		logger.Panic("Unloading an non-exist chunk")
// 	}
// 	// notify all viewers who are watching the chunk to unload the chunk
// 	for _, viewer := range c.viewers {
// 		viewer.ViewChunkUnload(pos)
// 	}
// 	// move the chunk to provider and save
// 	err := w.chunkProvider.PutChunk(pos, c.Chunk)
// 	if err != nil {
// 		logger.Error("Store chunk data error", zap.Error(err))
// 	}
// 	delete(w.chunks, pos)
// }

func (w *World) GetChunk(pos [2]int32) *LoadedChunk {
	return w.chunks[pos]
}

// BroadcastSwing sends an animation for the given player entity to all viewers in range.
func (w *World) BroadcastSwing(p *Player, animation byte) {
	cond := bvh.TouchPoint[vec3d, aabb3d](vec3d(p.Position))
	w.playerViews.Find(cond, func(n *playerViewNode) bool {
		if n.Value.Player == p {
			return true
		}
		n.Value.ViewAnimate(p.EntityID, animation)
		return true
	})
}

type LoadedChunk struct {
	sync.Mutex
	viewers []ChunkViewer
	// *level.Chunk
	*hpcworld.Chunk
	Pos level.ChunkPos
}

func (lc *LoadedChunk) AddViewer(v ChunkViewer) {
	lc.Lock()
	defer lc.Unlock()
	for _, v2 := range lc.viewers {
		if v2 == v {
			panic("append an exist viewer")
		}
	}
	lc.viewers = append(lc.viewers, v)
}

func (lc *LoadedChunk) RemoveViewer(v ChunkViewer) bool {
	lc.Lock()
	defer lc.Unlock()
	for i, v2 := range lc.viewers {
		if v2 == v {
			last := len(lc.viewers) - 1
			lc.viewers[i] = lc.viewers[last]
			lc.viewers = lc.viewers[:last]
			return true
		}
	}
	return false
}

func (lc *LoadedChunk) SetBlock(x, y, z int, block level.BlocksState) {
	lc.Lock()
	defer lc.Unlock()
	y += 64
	// lc.Chunk.Sections[y/16].SetBlock((x%16)*16*16+(y%16)*16+z%16, block)

	// get all >0 values

	tx := x % 16
	if tx < 0 {
		tx += 16
	}
	tz := z % 16
	if tz < 0 {
		tz += 16
	}

	lc.Chunk.Sections[y/16].SetBlock((y%16)*16*16+(tz%16)*16+tx%16, int32(block))

}

func (lc *LoadedChunk) UpdateToViewers() {
	lc.Lock()
	defer lc.Unlock()
	for _, v := range lc.viewers {
		// fmt.Println("update chunk to viewers", lc.Pos)
		v.ViewChunkLoad(lc.Pos, lc.Chunk)
	}
}

// updateRainbowInventory cycles rainbow colors through the top row of each player's inventory
func (w *World) updateRainbowInventory() {
	rainbowColors := []int32{
		209, // White Wool
		210, // Orange Wool
		211, // Magenta Wool
		212, // Light Blue Wool
		213, // Yellow Wool
		214, // Lime Wool
		215, // Pink Wool
		216, // Gray Wool
		217, // Light Gray Wool
		218, // Cyan Wool
		219, // Purple Wool
		220, // Blue Wool
		221, // Brown Wool
		222, // Green Wool
		223, // Red Wool
		224, // Black Wool
	}

	for c, p := range w.players {
		// Rotate colors for slots 9-17 (top inventory row)
		for i := 9; i < 10; i++ {
			colorIdx := (i - 9 + int(w.tickCount)) % len(rainbowColors)
			p.Inventory[i] = &ItemStack{
				ItemID: rainbowColors[colorIdx],
				Count:  1,
			}
			c.SendSetPlayerInventorySlot(int32(i), p.Inventory[i])
		}
	}
}
