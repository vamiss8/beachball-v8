// init canvas
const canvas = document.getElementById('gameCanvas');
const ctx = canvas.getContext('2d');

// hardcoded for now to match server logic, we'll make it dynamic later
canvas.width = 800;
canvas.height = 600;

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

// listen for keydown
window.addEventListener('keydown', (e) => {
    const key = e.key.toLowerCase();
    if (key === 'a' || key === 'd' || key === 'w' || key === ' ') {
        const inputKey = key === ' ' ? 'w' : key;
        keys[inputKey] = true;
        sendInput();
    }
});

// listen for keyup
window.addEventListener('keyup', (e) => {
    const key = e.key.toLowerCase();
    if (key === 'a' || key === 'd' || key === 'w' || key === ' ') {
        const inputKey = key === ' ' ? 'w' : key;
        keys[inputKey] = false;
        sendInput();
    }
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

    // draw ground
    ctx.fillStyle = '#eedd82';
    ctx.fillRect(0, canvas.height - 100, canvas.width, 100);

    // draw net
    ctx.fillStyle = '#333';
    ctx.fillRect(canvas.width / 2 - 5, canvas.height - 300, 10, 200);

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