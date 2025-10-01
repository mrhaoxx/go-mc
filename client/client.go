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

package client

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/mrhaoxx/go-mc/chat"
	"github.com/mrhaoxx/go-mc/data/packetid"
	"github.com/mrhaoxx/go-mc/level"
	"github.com/mrhaoxx/go-mc/net"
	pk "github.com/mrhaoxx/go-mc/net/packet"
	"github.com/mrhaoxx/go-mc/net/queue"
	"github.com/mrhaoxx/go-mc/server"
	"github.com/mrhaoxx/go-mc/world"
)

type Client struct {
	log      *zap.Logger
	conn     *net.Conn
	player   *world.Player
	world    *world.World
	queue    server.PacketQueue
	handlers []PacketHandler
	// pointer to the Player.Input
	*world.Inputs
}

type PacketHandler func(p pk.Packet, c *Client) error

func New(log *zap.Logger, conn *net.Conn, player *world.Player, world *world.World) *Client {
	return &Client{
		log:      log,
		conn:     conn,
		player:   player,
		world:    world,
		queue:    queue.NewChannelQueue[pk.Packet](256),
		handlers: defaultHandlers[:],
		Inputs:   &player.Inputs,
	}
}

func (c *Client) Start() {
	stopped := make(chan struct{}, 2)
	done := func() {
		stopped <- struct{}{}
	}
	// Exit when any error is thrown
	go c.startSend(done)
	go c.startReceive(done)
	<-stopped
}

func (c *Client) startSend(done func()) {
	defer done()
	for {
		p, ok := c.queue.Pull()
		if !ok {
			return
		}
		err := c.conn.WritePacket(p)
		if err != nil {
			c.log.Debug("Send packet fail", zap.Error(err))
			return
		}
		if packetid.ClientboundPacketID(p.ID) == packetid.ClientboundDisconnect {
			return
		}
	}
}

func (c *Client) startReceive(done func()) {
	defer done()
	var packet pk.Packet
	for {
		err := c.conn.ReadPacket(&packet)
		if err != nil {
			c.log.Debug("Receive packet fail", zap.Error(err))
			return
		}
		if packet.ID < 0 || packet.ID >= int32(len(c.handlers)) {
			c.log.Debug("Invalid packet id", zap.Int32("id", packet.ID), zap.Int("len", len(packet.Data)))
			return
		}
		if handler := c.handlers[packet.ID]; handler != nil {
			err = handler(packet, c)
			if err != nil {
				c.log.Error("Handle packet error", zap.Int32("id", packet.ID), zap.Error(err))
				return
			}
		} else {
			c.log.Info("Unhandled packet id", zap.Int32("id", packet.ID), zap.Int("len", len(packet.Data)), zap.String("type", string(packetid.ServerboundPacketID(packet.ID).String())))
		}
	}
}

func (c *Client) AddHandler(id packetid.ServerboundPacketID, handler PacketHandler) {
	c.handlers[id] = handler
}
func (c *Client) GetPlayer() *world.Player { return c.player }

var defaultHandlers = [packetid.ServerboundPacketIDGuard]PacketHandler{
	packetid.ServerboundAcceptTeleportation:  clientAcceptTeleportation,
	packetid.ServerboundClientInformation:    clientInformation,
	packetid.ServerboundMovePlayerPos:        clientMovePlayerPos,
	packetid.ServerboundMovePlayerPosRot:     clientMovePlayerPosRot,
	packetid.ServerboundMovePlayerRot:        clientMovePlayerRot,
	packetid.ServerboundMovePlayerStatusOnly: clientMovePlayerStatusOnly,
	packetid.ServerboundMoveVehicle:          clientMoveVehicle,
	packetid.ServerboundChatCommand: func(p pk.Packet, c *Client) error {
		var command pk.String
		if err := p.Scan(&command); err != nil {
			return err
		}
		fmt.Println("command", command)

		var splits = strings.Split(string(command), " ")
		if len(splits) < 1 {
			c.SendSystemChat(chat.Message{
				Text: "Commands Separated by Space",
			}, false)
			return nil
		}
		var cmd = splits[0]
		var args = splits[1:]
		switch cmd {
		case "ping":
			c.SendSystemChat(chat.Message{
				Text: "Pong!",
			}, false)
		case "tp":
			if len(args) != 3 {
				c.SendSystemChat(chat.Message{
					Text: "Usage: /tp <x> <y> <z>",
				}, false)
				return nil
			}
			var x, y, z int
			fmt.Sscanf(args[0], "%d", &x)
			fmt.Sscanf(args[1], "%d", &y)
			fmt.Sscanf(args[2], "%d", &z)
			c.SendSystemChat(chat.Message{
				Text: "Teleporting to " + args[0] + " " + args[1] + " " + args[2],
			}, false)
			c.player.Position[0] = float64(x)
			c.player.Position[1] = float64(y)
			c.player.Position[2] = float64(z)
			c.SendPlayerPosition(c.player.Position, c.player.Rotation)
			return nil

		case "setblock":

			if len(args) != 4 {
				c.SendSystemChat(chat.Message{
					Text: "Usage: /setblock <x> <y> <z> <block>",
				}, false)
				return nil
			}

			var x, y, z int
			fmt.Sscanf(args[0], "%d", &x)
			fmt.Sscanf(args[1], "%d", &y)
			fmt.Sscanf(args[2], "%d", &z)
			var block int
			fmt.Sscanf(args[3], "%d", &block)
			c.SendSystemChat(chat.Message{
				Text: "Setting block to " + args[3] + " at " + args[0] + " " + args[1] + " " + args[2],
			}, false)
			c.world.GetChunk([2]int32{int32(x / 16), int32(z / 16)}).SetBlock(x, y, z, level.BlocksState(block))
			return nil

		case "fill":
			if len(args) != 7 {
				c.SendSystemChat(chat.Message{
					Text: "Usage: /fill <x1> <y1> <z1> <x2> <y2> <z2> <block>",
				}, false)
				return nil
			}

			var x1, y1, z1, x2, y2, z2 int
			fmt.Sscanf(args[0], "%d", &x1)
			fmt.Sscanf(args[1], "%d", &y1)
			fmt.Sscanf(args[2], "%d", &z1)
			fmt.Sscanf(args[3], "%d", &x2)
			fmt.Sscanf(args[4], "%d", &y2)
			fmt.Sscanf(args[5], "%d", &z2)
			var block int
			fmt.Sscanf(args[6], "%d", &block)

			// Ensure coordinates are in the right order (min to max)
			if x1 > x2 {
				x1, x2 = x2, x1
			}
			if y1 > y2 {
				y1, y2 = y2, y1
			}
			if z1 > z2 {
				z1, z2 = z2, z1
			}

			// Calculate the volume
			volume := (x2 - x1 + 1) * (y2 - y1 + 1) * (z2 - z1 + 1)

			c.SendSystemChat(chat.Message{
				Text: fmt.Sprintf("Filling %d blocks from (%d,%d,%d) to (%d,%d,%d) with block %s", volume, x1, y1, z1, x2, y2, z2, args[6]),
			}, false)

			// Fill the region
			for x := x1; x <= x2; x++ {
				for y := y1; y <= y2; y++ {
					for z := z1; z <= z2; z++ {
						var ck = c.world.GetChunk([2]int32{int32(x / 16), int32(z / 16)})
						if ck == nil {
							c.SendSystemChat(chat.Message{
								Text: fmt.Sprintf("Chunk not found at (%d,%d)", x, z),
							}, false)
							continue
						}

						ck.SetBlock(x, y, z, level.BlocksState(block))
					}
				}
			}

			c.SendSystemChat(chat.Message{
				Text: fmt.Sprintf("Successfully filled %d blocks", volume),
			}, false)
			return nil

		default:
			c.SendSystemChat(chat.Message{
				Text: "Unknown command: " + cmd,
			}, false)
		}
		return nil

	},
	packetid.ServerboundChunkBatchReceived: func(p pk.Packet, c *Client) error {
		var chunkBatch pk.Float
		if err := p.Scan(&chunkBatch); err != nil {
			return err
		}
		fmt.Println("Client: Chunk batch Received", chunkBatch)
		return nil
	},
	packetid.ServerboundClientTickEnd: func(p pk.Packet, c *Client) error {

		// fmt.Println("Client: Client tick end")
		return nil
	},
	packetid.ServerboundPlayerInput: func(p pk.Packet, c *Client) error {
		var playerInput pk.UnsignedByte
		if err := p.Scan(&playerInput); err != nil {
			return err
		}
		// to string
		var playerInputString string

		// Hex Mask	Field
		// 0x01	Forward
		// 0x02	Backward
		// 0x04	Left
		// 0x08	Right
		// 0x10	Jump
		// 0x20	Sneak
		// 0x40	Sprint

		for i := 0; i < 6; i++ {
			if playerInput&(1<<i) != 0 {
				playerInputString += fmt.Sprintf("%s ", []string{"Forward", "Backward", "Left", "Right", "Jump", "Sneak", "Sprint"}[i])
			}
		}

		fmt.Println("Client: Player input", playerInputString)
		c.log.Info("Client: Player input", zap.String("input", playerInputString), zap.String("name", c.player.Name))
		return nil
	},
	packetid.ServerboundSwing: func(p pk.Packet, c *Client) error {
		var swing pk.VarInt
		if err := p.Scan(&swing); err != nil {
			return err
		}
		c.log.Info("Client: Swing", zap.Int32("swing", int32(swing)), zap.String("name", c.player.Name))
		return nil
	},
}
