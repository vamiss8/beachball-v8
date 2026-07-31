// canvas rendering. draws whatever world it is handed and nothing else: no
// rules, no prediction, no state of its own beyond cosmetics.

// radians of fake ball spin per unit of horizontal speed per tick
const BALL_SPIN_PER_TICK = 0.01;

// css pixels left free around the canvas so its border and shadow have room
const VIEWPORT_MARGIN = 32;

const COLORS = {
  skyTop: '#4fc3f7',
  skyBottom: '#b3e5fc',
  sand: '#e6c288',
  sandShade: '#d4ad6f',
  net: '#fafafa',
  netPost: '#8d6e63',
  ball: '#ffeb3b',
  ballStripe: '#f57f17',
  left: '#43a047',
  right: '#1e88e5',
  outline: 'rgba(0, 0, 0, 0.35)',
  text: '#ffffff',
  textShadow: 'rgba(0, 0, 0, 0.55)',
};

export class Renderer {
  constructor(canvas, arena) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.arena = arena;

    // cosmetic only: the server does not track ball spin, so it is faked
    // locally from horizontal speed
    this.ballSpin = 0;

    this.lastTick = null;

    this.resize();
    window.addEventListener('resize', () => this.resize());
  }

  // resize fits the canvas itself to the arena's aspect ratio instead of
  // filling the window and letterboxing inside. the canvas box is then the
  // playfield exactly, so the border drawn in css hugs the court and both
  // players still see the same field whatever their window size is
  resize() {
    const dpr = window.devicePixelRatio || 1;

    // a window narrower than the margin must not collapse the canvas to zero
    const availableWidth = Math.max(window.innerWidth - VIEWPORT_MARGIN, 1);
    const availableHeight = Math.max(window.innerHeight - VIEWPORT_MARGIN, 1);

    const fit = Math.min(availableWidth / this.arena.width, availableHeight / this.arena.height);

    // rounded so the backing buffer lands on whole device pixels, then the
    // scale is taken back from the rounded size to stay exact
    this.viewWidth = Math.round(this.arena.width * fit);
    this.viewHeight = Math.round(this.arena.height * fit);
    this.scale = this.viewWidth / this.arena.width;

    this.canvas.style.width = `${this.viewWidth}px`;
    this.canvas.style.height = `${this.viewHeight}px`;
    this.canvas.width = Math.round(this.viewWidth * dpr);
    this.canvas.height = Math.round(this.viewHeight * dpr);

    this.dpr = dpr;
  }

  draw(world, view) {
    const ctx = this.ctx;

    // the canvas is sized in device pixels, so filling it with its own
    // width and height under the dpr transform covers up to four times the
    // area actually on screen
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, this.viewWidth, this.viewHeight);
    ctx.fillStyle = '#102027';
    ctx.fillRect(0, 0, this.viewWidth, this.viewHeight);

    ctx.save();
    ctx.scale(this.scale, this.scale);

    this.drawBackground();

    if (world) {
      this.drawNet();
      for (const player of Object.values(world.players)) {
        this.drawPlayer(player, player.id === view.playerId);
      }
      this.drawBall(world.ball, this.ticksSinceLastFrame(world.tick));
    }

    ctx.restore();

    if (world) {
      this.drawScore(world);
      this.drawBanner(world, view);
    }
    this.drawStatus(view);
  }

  drawBackground() {
    const ctx = this.ctx;
    const { width, height, groundY } = this.arena;

    const sky = ctx.createLinearGradient(0, 0, 0, groundY);
    sky.addColorStop(0, COLORS.skyTop);
    sky.addColorStop(1, COLORS.skyBottom);
    ctx.fillStyle = sky;
    ctx.fillRect(0, 0, width, groundY);

    ctx.fillStyle = COLORS.sand;
    ctx.fillRect(0, groundY, width, height - groundY);
    ctx.fillStyle = COLORS.sandShade;
    ctx.fillRect(0, groundY, width, 8);
  }

  drawNet() {
    const ctx = this.ctx;
    const { netX, netY, netWidth, netHeight, groundY } = this.arena;

    ctx.fillStyle = COLORS.netPost;
    ctx.fillRect(netX, netY, netWidth, groundY - netY);

    ctx.fillStyle = COLORS.net;
    ctx.fillRect(netX, netY, netWidth, netHeight * 0.75);
    ctx.fillStyle = COLORS.netPost;
    ctx.fillRect(netX - 2, netY - 6, netWidth + 4, 10);
  }

  drawPlayer(player, isSelf) {
    const ctx = this.ctx;
    const { playerWidth, playerHeight } = this.arena;

    const cx = player.pos.x + playerWidth / 2;
    const cy = player.pos.y + playerHeight / 2;

    ctx.save();
    ctx.translate(cx, cy);
    ctx.rotate(player.rotation);

    ctx.fillStyle = player.side === 'left' ? COLORS.left : COLORS.right;
    roundedRect(ctx, -playerWidth / 2, -playerHeight / 2, playerWidth, playerHeight, 24);
    ctx.fill();

    // the local player gets an outline, otherwise it is easy to lose track of
    // which body is yours in the middle of a rally
    if (isSelf) {
      ctx.lineWidth = 6;
      ctx.strokeStyle = COLORS.text;
      ctx.stroke();
    }

    // eyes, purely so the shape reads as a character and shows its rotation
    ctx.fillStyle = '#ffffff';
    const eyeOffset = player.side === 'left' ? 14 : -14;
    circle(ctx, eyeOffset - 12, -18, 11);
    circle(ctx, eyeOffset + 16, -18, 11);
    ctx.fillStyle = '#1b1b1b';
    circle(ctx, eyeOffset - 12, -18, 5);
    circle(ctx, eyeOffset + 16, -18, 5);

    ctx.restore();
  }

  // how far the render clock moved since the previous frame, in ticks. the
  // interpolated world already carries a fractional tick, so nothing else
  // has to be timed here
  ticksSinceLastFrame(tick) {
    const previous = this.lastTick;
    this.lastTick = tick;
    // first frame, and a reconnect that restarts the counter, both spin by
    // nothing rather than by a garbage delta
    if (previous === null || tick < previous) return 0;
    return tick - previous;
  }

  drawBall(ball, ticks) {
    const ctx = this.ctx;

    // velocity is per tick, so the spin has to advance per tick too:
    // accumulating once per animation frame made the ball spin twice as
    // fast on a 120hz screen as on a 60hz one
    this.ballSpin += ball.velocity.x * BALL_SPIN_PER_TICK * ticks;

    ctx.save();
    ctx.translate(ball.pos.x, ball.pos.y);
    ctx.rotate(this.ballSpin);

    ctx.fillStyle = COLORS.ball;
    circle(ctx, 0, 0, ball.radius);

    ctx.strokeStyle = COLORS.ballStripe;
    ctx.lineWidth = 6;
    for (let i = 0; i < 3; i++) {
      ctx.beginPath();
      ctx.arc(0, 0, ball.radius * 0.72, (i * 2 * Math.PI) / 3, (i * 2 * Math.PI) / 3 + 1.1);
      ctx.stroke();
    }

    ctx.strokeStyle = COLORS.outline;
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.arc(0, 0, ball.radius, 0, Math.PI * 2);
    ctx.stroke();

    ctx.restore();
  }

  drawScore(world) {
    const ctx = this.ctx;
    const centerX = this.viewWidth / 2;
    const y = 60 * this.scale;

    // scaled with the court, otherwise the text swallows a small window
    text(ctx, `${world.score.left} : ${world.score.right}`, centerX, y, 64 * this.scale, 'center');
  }

  drawBanner(world, view) {
    const ctx = this.ctx;
    const centerX = this.viewWidth / 2;
    const centerY = this.viewHeight / 2.6;

    let message = null;
    if (world.phase === 'serve') {
      message = `${world.serveSide === 'left' ? 'ЗЕЛЁНЫЕ' : 'СИНИЕ'} ПОДАЮТ`;
    } else if (world.phase === 'scored') {
      message = 'ОЧКО';
    } else if (world.phase === 'finished') {
      const won = world.winner === view.side;
      message = view.spectator ? 'МАТЧ ОКОНЧЕН' : won ? 'ПОБЕДА' : 'ПОРАЖЕНИЕ';
    }

    if (message) text(ctx, message, centerX, centerY, 52 * this.scale, 'center');
  }

  drawStatus(view) {
    const ctx = this.ctx;
    const x = 24 * this.scale;
    const y = 40 * this.scale;
    const size = 28 * this.scale;

    if (view.status !== 'connected') {
      text(ctx, view.status === 'connecting' ? 'подключение…' : 'соединение потеряно', x, y, size, 'left');
      return;
    }
    if (view.spectator) {
      text(ctx, 'наблюдатель: оба места заняты', x, y, size, 'left');
    }
  }
}

function roundedRect(ctx, x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

function circle(ctx, x, y, r) {
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fill();
}

function text(ctx, value, x, y, size, align) {
  ctx.save();
  ctx.font = `bold ${size}px system-ui, sans-serif`;
  ctx.textAlign = align;
  ctx.textBaseline = 'middle';
  ctx.lineWidth = 6;
  ctx.strokeStyle = COLORS.textShadow;
  ctx.strokeText(value, x, y);
  ctx.fillStyle = COLORS.text;
  ctx.fillText(value, x, y);
  ctx.restore();
}
