// entry point: wires the socket, the keyboard and the renderer together and
// drives the frame loop.

import { Connection } from './net.js';
import { Input } from './input.js';
import { SnapshotBuffer } from './interpolate.js';
import { Renderer } from './render.js';

const canvas = document.getElementById('game');
const input = new Input();

// everything the renderer needs that is not part of the world snapshot
const view = {
  status: 'connecting',
  playerId: null,
  side: null,
  spectator: false,
};

let renderer = null;
let buffer = null;

const connection = new Connection({
  onStatus: (status) => {
    view.status = status;
  },

  // the arena and tick rate arrive with the welcome message, so the client
  // never hardcodes numbers the server owns
  onWelcome: (welcome) => {
    view.playerId = welcome.playerId;
    view.side = welcome.side;
    view.spectator = welcome.spectator;

    renderer = new Renderer(canvas, welcome.arena);
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
  renderer.draw(buffer.sample(dt), view);
}

requestAnimationFrame(frame);
