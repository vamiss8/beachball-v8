// init canvas
const canvas = document.getElementById('gameCanvas');
const ctx = canvas.getContext('2d');

// handle resize
function resize() {
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
}
window.addEventListener('resize', resize);
resize();

// placeholder game state
let ball = { x: canvas.width / 4, y: 100, radius: 20 };

// game loop
function update() {
    // simple gravity logic for testing
    ball.y += 2;
    if (ball.y > canvas.height - 100) {
        ball.y = 100;
    }
}

function draw() {
    // clear screen
    ctx.clearRect(0, 0, canvas.width, canvas.height);

    // draw ground
    ctx.fillStyle = '#eedd82';
    ctx.fillRect(0, canvas.height - 100, canvas.width, 100);

    // draw net
    ctx.fillStyle = '#333';
    ctx.fillRect(canvas.width / 2 - 5, canvas.height - 300, 10, 200);

    // draw ball
    ctx.beginPath();
    ctx.arc(ball.x, ball.y, ball.radius, 0, Math.PI * 2);
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
    socket.send("hello from 2d client");
};

socket.onmessage = (event) => {
    console.log("server reply:", event.data);
};