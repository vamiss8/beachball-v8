// init canvas
const canvas = document.getElementById('gameCanvas');
const ctx = canvas.getContext('2d');

// logical resolution (fixed aspect ratio 16:9)
const logicalWidth = 1600;
const logicalHeight = 900;
canvas.width = logicalWidth;
canvas.height = logicalHeight;

// scale canvas to fit window automatically
function resize() {
    const scale = Math.min(
        window.innerWidth / logicalWidth, 
        window.innerHeight / logicalHeight
    );
    canvas.style.width = `${logicalWidth * scale}px`;
    canvas.style.height = `${logicalHeight * scale}px`;
}
window.addEventListener('resize', resize);
resize();

// local state placeholders (will be overwritten by server)
let serverState = null;

// input tracking
const keys = { a: false, d: false, w: false };

// init websocket
const socket = new WebSocket("ws://localhost:8080/ws");

socket.onopen = () => {
    console.log("connected to server");
};

// receive state from server
socket.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.type === 'state') {
        serverState = data.state;
    }
};

// listen for keydown (fixed layout issue using e.code)
window.addEventListener('keydown', (e) => {
    if (e.code === 'KeyA') keys.a = true;
    if (e.code === 'KeyD') keys.d = true;
    if (e.code === 'KeyW' || e.code === 'Space') keys.w = true;
    sendInput();
});

// listen for keyup
window.addEventListener('keyup', (e) => {
    if (e.code === 'KeyA') keys.a = false;
    if (e.code === 'KeyD') keys.d = false;
    if (e.code === 'KeyW' || e.code === 'Space') keys.w = false;
    sendInput();
});

// clear inputs on window blur (fixed sticky keys)
window.addEventListener('blur', () => {
    keys.a = false;
    keys.d = false;
    keys.w = false;
    sendInput();
});

// send input to server
function sendInput() {
    if (socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'input', keys: keys }));
    }
}

// render frame
function draw() {
    // clear screen
    ctx.clearRect(0, 0, canvas.width, canvas.height);

    // draw ground (высота 150, как на сервере)
    ctx.fillStyle = '#eedd82';
    ctx.fillRect(0, canvas.height - 150, canvas.width, 150);

    // draw net (смещение 500 и высота 350, как на сервере)
    ctx.fillStyle = '#333';
    ctx.fillRect(canvas.width / 2 - 10, canvas.height - 500, 20, 350);

    // draw entities if state is received
    if (serverState) {
        // draw all players
        for (const id in serverState.players) {
            const p = serverState.players[id];
            ctx.fillStyle = p.color;
            ctx.fillRect(p.pos.x, p.pos.y, p.width, p.height);
        }

        // draw ball
        const b = serverState.ball;
        ctx.beginPath();
        ctx.arc(b.pos.x, b.pos.y, b.radius, 0, Math.PI * 2);
        ctx.fillStyle = '#ff4444';
        ctx.fill();
        ctx.closePath();
    }
}

// main loop (render only)
function loop() {
    draw();
    requestAnimationFrame(loop);
}

// start renderer
loop();