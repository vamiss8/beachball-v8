// entry point: wires the socket, the keyboard and the renderer together and
// drives the frame loop.

import { Connection } from './net.js';
import { Input } from './input.js';
import { SnapshotBuffer } from './interpolate.js';
import { Renderer } from './render.js';
import { RoomBar } from './ui.js';
import { LobbyPanel } from './lobby.js';

const canvas = document.getElementById('game');
const input = new Input();
const roomBar = new RoomBar(document.getElementById('room-bar'));
const lobby = new LobbyPanel(document.getElementById('lobby'), {
  onChange: (state) => connection.sendLobby(state),
});

// everything the renderer needs that is not part of the world snapshot
const view = {
  status: 'connecting',
  playerId: null,
  spectator: false,
  pointsToWin: null,
};

let renderer = null;
let buffer = null;

const connection = new Connection({
  onStatus: (status) => {
    view.status = status;
    if (status !== 'connected') roomBar.clearPing();
  },

  onPing: (rtt) => roomBar.showPing(rtt),

  // the arena and tick rate arrive with the welcome message, so the client
  // never hardcodes numbers the server owns
  onWelcome: (welcome) => {
    // welcome.side is deliberately not kept: every player carries their own
    // side in the snapshot and the renderer colours the court from that, so
    // a copy here would only be a second source for the same fact
    view.playerId = welcome.playerId;
    view.spectator = welcome.spectator;
    view.pointsToWin = welcome.arena.pointsToWin;

    // the server names the room, including when it opened a fresh one for us
    roomBar.show(welcome.roomId);

    // rebuilt only once: the arena never changes, and a fresh Renderer on
    // every reconnect would stack up another resize listener each time
    renderer ??= new Renderer(canvas, welcome.arena);
    buffer = new SnapshotBuffer(welcome.arena.tickRate);
  },

  onSnapshot: (world) => {
    if (buffer) buffer.push(world);
  },
});

connection.connect();

let lastFrame = performance.now();

function frame(now) {
  requestAnimationFrame(frame);

  // clamped because a backgrounded tab hands back a huge delta on return
  const dt = Math.min((now - lastFrame) / 1000, 0.25);
  lastFrame = now;

  connection.sendInput(input.keys);

  if (!renderer) return;

  const world = buffer.sample(dt);
  if (world) lobby.update(world, view);
  renderer.draw(world, view);
}

requestAnimationFrame(frame);
