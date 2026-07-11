// init canvas
const canvas = document.getElementById('gameCanvas');
const ctx = canvas.getContext('2d');

let groundY = 0;
const gravity = 0.6;

// handle resize
function resize() {
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
    groundY = canvas.height - 100;
}
window.addEventListener('resize', resize);
resize();

// game state
const state = {
    ball: { 
        x: canvas.width / 4, 
        y: 100, 
        radius: 20 
    },
    player: {
        x: canvas.width / 4,
        y: 100,
        width: 50,
        height: 50,
        vx: 0, // velocity x
        vy: 0, // velocity y
        speed: 7,
        jumpPower: -12,
        isGrounded: false
    }
};

// input tracking
const keys = {
    a: false,
    d: false,
    w: false
};

window.addEventListener('keydown', (e) => {
    if (e.key === 'a' || e.key === 'A') keys.a = true;
    if (e.key === 'd' || e.key === 'D') keys.d = true;
    if (e.key === 'w' || e.key === 'W' || e.key === ' ') keys.w = true;
});

window.addEventListener('keyup', (e) => {
    if (e.key === 'a' || e.key === 'A') keys.a = false;
    if (e.key === 'd' || e.key === 'D') keys.d = false;
    if (e.key === 'w' || e.key === 'W' || e.key === ' ') keys.w = false;
});

// game loop
function update() {
    // placeholder ball physics
    state.ball.y += 2;
    if (state.ball.y > groundY - state.ball.radius) {
        state.ball.y = 100;
    }

    // player horizontal movement
    if (keys.a) {
        state.player.vx = -state.player.speed;
    } else if (keys.d) {
        state.player.vx = state.player.speed;
    } else {
        state.player.vx = 0;
    }

    // player jump
    if (keys.w && state.player.isGrounded) {
        state.player.vy = state.player.jumpPower;
        state.player.isGrounded = false;
    }

    // apply gravity to player
    state.player.vy += gravity;
    
    // update player position
    state.player.x += state.player.vx;
    state.player.y += state.player.vy;

    // floor collision for player
    if (state.player.y + state.player.height >= groundY) {
        state.player.y = groundY - state.player.height;
        state.player.vy = 0;
        state.player.isGrounded = true;
    }
    
    // basic screen bounds for player
    if (state.player.x < 0) state.player.x = 0;
    if (state.player.x + state.player.width > canvas.width) {
        state.player.x = canvas.width - state.player.width;
    }
}

function draw() {
    // clear screen
    ctx.clearRect(0, 0, canvas.width, canvas.height);

    // draw ground
    ctx.fillStyle = '#eedd82';
    ctx.fillRect(0, groundY, canvas.width, 100);

    // draw net
    ctx.fillStyle = '#333';
    ctx.fillRect(canvas.width / 2 - 5, groundY - 200, 10, 200);

    // draw player (blue square)
    ctx.fillStyle = '#4444ff';
    ctx.fillRect(state.player.x, state.player.y, state.player.width, state.player.height);

    // draw ball
    ctx.beginPath();
    ctx.arc(state.ball.x, state.ball.y, state.ball.radius, 0, Math.PI * 2);
    ctx.fillStyle = '#ff4444';
    ctx.fill();
    ctx.closePath();
}

function loop() {
    update();
    draw();
    requestAnimationFrame(loop);
}

// start game
loop();

// init websocket
const socket = new WebSocket("ws://localhost:8080/ws");

socket.onopen = () => {
    console.log("connected to server");
    // socket.send("hello from 2d client");
};

socket.onmessage = (event) => {
    console.log("server reply:", event.data);
};