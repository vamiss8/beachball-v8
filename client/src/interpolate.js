// snapshot buffering and interpolation.
//
// the server simulates at a fixed tick rate, but snapshots arrive with
// network jitter. rendering each one the moment it lands looks stuttery, so
// the client deliberately renders slightly in the past and blends between the
// two snapshots surrounding that moment.

// how far behind the newest snapshot we render. one hundred milliseconds is
// enough to ride out normal jitter and a couple of dropped frames, and small
// enough that the input lag stays unnoticeable
const DELAY_TICKS = 6;

// snapshots older than this are useless, keep the buffer bounded
const MAX_SNAPSHOTS = 60;

// how aggressively the render clock is pulled back toward its target. a hard
// snap would be visible as a jump, so the drift is corrected gradually
const CLOCK_CORRECTION = 0.08;

// past this gap something went badly wrong (tab was backgrounded, connection
// froze) and smoothing is pointless, so the clock jumps instead
const RESYNC_THRESHOLD_TICKS = 30;

export class SnapshotBuffer {
  constructor(tickRate) {
    this.tickRate = tickRate;
    this.snapshots = [];
    this.renderTick = null;
  }

  push(world) {
    const last = this.snapshots[this.snapshots.length - 1];
    // the server may reorder nothing over tcp, but a reconnect restarts the
    // tick counter, so an out-of-order tick means a new world entirely
    if (last && world.tick <= last.tick) {
      this.snapshots = [];
      this.renderTick = null;
    }

    this.snapshots.push(world);
    if (this.snapshots.length > MAX_SNAPSHOTS) {
      this.snapshots.shift();
    }
  }

  // sample advances the render clock by dt and returns the world as it should
  // be drawn right now, or null while the buffer is still filling up
  sample(dt) {
    if (this.snapshots.length === 0) return null;

    const newest = this.snapshots[this.snapshots.length - 1];
    const oldest = this.snapshots[0];
    const target = newest.tick - DELAY_TICKS;

    if (this.renderTick === null || Math.abs(target - this.renderTick) > RESYNC_THRESHOLD_TICKS) {
      this.renderTick = target;
    } else {
      this.renderTick += dt * this.tickRate;
      this.renderTick += (target - this.renderTick) * CLOCK_CORRECTION;
    }

    // never extrapolate past what the server has actually told us
    this.renderTick = Math.min(Math.max(this.renderTick, oldest.tick), newest.tick);

    const [a, b, t] = this.bracket(this.renderTick);
    return lerpWorld(a, b, t);
  }

  // bracket finds the two snapshots surrounding a tick and the blend factor
  bracket(tick) {
    for (let i = this.snapshots.length - 1; i > 0; i--) {
      const b = this.snapshots[i];
      const a = this.snapshots[i - 1];
      if (a.tick <= tick && tick <= b.tick) {
        const span = b.tick - a.tick;
        return [a, b, span === 0 ? 0 : (tick - a.tick) / span];
      }
    }
    const only = this.snapshots[0];
    return [only, only, 0];
  }
}

function lerp(a, b, t) {
  return a + (b - a) * t;
}

// blends two snapshots. everything the player sees moving is interpolated;
// discrete state is taken from the older snapshot so that the score and the
// phase banner stay in sync with the ball being drawn
function lerpWorld(a, b, t) {
  const world = {
    tick: lerp(a.tick, b.tick, t),
    ball: lerpBall(a.ball, b.ball, t),
    players: {},
    score: a.score,
    phase: a.phase,
    serveSide: a.serveSide,
    winner: a.winner,
  };

  for (const [id, playerA] of Object.entries(a.players)) {
    const playerB = b.players[id];
    // a player who left between the two snapshots is drawn at their last
    // known position rather than vanishing mid-blend
    world.players[id] = playerB ? lerpPlayer(playerA, playerB, t) : playerA;
  }

  return world;
}

function lerpBall(a, b, t) {
  return {
    ...a,
    pos: { x: lerp(a.pos.x, b.pos.x, t), y: lerp(a.pos.y, b.pos.y, t) },
  };
}

function lerpPlayer(a, b, t) {
  return {
    ...a,
    pos: { x: lerp(a.pos.x, b.pos.x, t), y: lerp(a.pos.y, b.pos.y, t) },
    rotation: lerpRotation(a.rotation, b.rotation, t),
    isBlocking: b.isBlocking,
    isJumping: b.isJumping,
  };
}

// a full spin resets from nearly 2pi back to 0 on the server, and blending
// across that reset would spin the player backwards for a frame
function lerpRotation(a, b, t) {
  return Math.abs(b - a) > Math.PI ? b : lerp(a, b, t);
}
