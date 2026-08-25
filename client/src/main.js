// entry point: wires the socket, the keyboard and the renderer together and
// drives the frame loop.

import { Connection } from './net.js';
import { Input } from './input.js';
import { SnapshotBuffer } from './interpolate.js';
import { Renderer } from './render.js';
import { RoomBar } from './ui.js';
import { LobbyPanel } from './lobby.js';
import { Prediction } from './prediction.js';

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
let prediction = null;

const connection = new Connection({
  onStatus: (status) => {
    view.status = status;
    if (status !== 'connected') {
      roomBar.clearPing();
      // a reconnect is a new player as far as the server is concerned, so
      // nothing predicted for the old one still applies
      if (prediction) prediction.forget();
    }
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

    // rebuilt only once: arena never changes, and a fresh Renderer on
    // every reconnect would stack up another resize listener each time
    renderer ??= new Renderer(canvas, welcome.arena);
    buffer = new SnapshotBuffer(welcome.arena.tickRate);

    // spectators have no player of their own to predict
    prediction = welcome.spectator ? null : new Prediction(welcome.arena, welcome.tuning);
  },

  onSnapshot: (world) => {
    if (buffer) buffer.push(world);

    // reconciled against the raw snapshot, not the interpolated one: this is
    // what the server actually decided, while the interpolated world is a
    // blend rendered deliberately in the past
    const me = prediction && world.players[view.playerId];
    if (me) prediction.reconcile(me);
  },
});

connection.connect();

let lastFrame = performance.now();

function frame(now) {
  requestAnimationFrame(frame);

  // clamped because a backgrounded tab hands back a huge delta on return
  const dt = Math.min((now - lastFrame) / 1000, 0.25);
  lastFrame = now;

  // one input per fixed tick, however fast this screen happens to redraw
  if (prediction) {
    for (const pending of prediction.advance(dt, input.keys)) {
      connection.sendInput(pending);
    }
  }

  if (!renderer) return;

  const world = buffer.sample(dt);
  if (world) {
    lobby.update(world, view);
    // your own player comes from the prediction instead of the delayed
    // snapshot, which is the whole point: it answers the keyboard now
    const predicted = prediction && prediction.view();
    if (predicted && world.players[view.playerId]) {
      world.players[view.playerId] = { ...world.players[view.playerId], ...predicted };
    }
  }
  renderer.draw(world, view);
}

requestAnimationFrame(frame);
