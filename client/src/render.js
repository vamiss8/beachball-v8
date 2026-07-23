// canvas rendering. draws whatever world it is handed and nothing else: no
// rules, no prediction, no state of its own beyond cosmetics.

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

    this.resize();
    window.addEventListener('resize', () => this.resize());
  }

  // resize keeps the arena aspect ratio and letterboxes the rest, so both
  // players see exactly the same playfield whatever their window size is
  resize() {
    const dpr = window.devicePixelRatio || 1;
    const width = window.innerWidth;
    const height = window.innerHeight;

    this.canvas.width = width * dpr;
    this.canvas.height = height * dpr;
    this.canvas.style.width = `${width}px`;
    this.canvas.style.height = `${height}px`;

    this.scale = Math.min(width / this.arena.width, height / this.arena.height);
    this.offsetX = (width - this.arena.width * this.scale) / 2;
    this.offsetY = (height - this.arena.height * this.scale) / 2;
    this.dpr = dpr;
  }

  draw(world, view) {
    const ctx = this.ctx;

    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    ctx.fillStyle = '#102027';
    ctx.fillRect(0, 0, this.canvas.width, this.canvas.height);

    ctx.save();
    ctx.translate(this.offsetX, this.offsetY);
    ctx.scale(this.scale, this.scale);

    this.drawBackground();

    if (world) {
      this.drawNet();
      for (const player of Object.values(world.players)) {
        this.drawPlayer(player, player.id === view.playerId);
      }
      this.drawBall(world.ball);
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

  drawBall(ball) {
    const ctx = this.ctx;

    this.ballSpin += ball.velocity.x * 0.01;

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
    const centerX = this.offsetX + (this.arena.width * this.scale) / 2;
    const y = this.offsetY + 60 * this.scale;

    text(ctx, `${world.score.left} : ${world.score.right}`, centerX, y, 64, 'center');
  }

  drawBanner(world, view) {
    const ctx = this.ctx;
    const centerX = this.offsetX + (this.arena.width * this.scale) / 2;
    const centerY = this.offsetY + (this.arena.height * this.scale) / 2.6;

    let message = null;
    if (world.phase === 'serve') {
      message = `${world.serveSide === 'left' ? 'ЗЕЛЁНЫЕ' : 'СИНИЕ'} ПОДАЮТ`;
    } else if (world.phase === 'scored') {
      message = 'ОЧКО';
    } else if (world.phase === 'finished') {
      const won = world.winner === view.side;
      message = view.spectator ? 'МАТЧ ОКОНЧЕН' : won ? 'ПОБЕДА' : 'ПОРАЖЕНИЕ';
    }

    if (message) text(ctx, message, centerX, centerY, 52, 'center');
  }

  drawStatus(view) {
    const ctx = this.ctx;
    const x = this.offsetX + 24;
    const y = this.offsetY + 40;

    if (view.status !== 'connected') {
      text(ctx, view.status === 'connecting' ? 'подключение…' : 'соединение потеряно', x, y, 28, 'left');
      return;
    }
    if (view.spectator) {
      text(ctx, 'наблюдатель: оба места заняты', x, y, 28, 'left');
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
