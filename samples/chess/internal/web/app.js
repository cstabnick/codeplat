"use strict";

// The page talks to the REST API on the same origin:
//   GET  /api/games                    -> Game[]
//   POST /api/games                    -> Game (201)
//   GET  /api/games/{id}               -> Game
//   POST /api/games/{id}/moves         -> Game (includes the opponent's reply)
//   POST /api/games/{id}/resign        -> Game
// Errors come back as {"error": "..."}.

const PIECES = {
  K: "♔", Q: "♕", R: "♖", B: "♗", N: "♘", P: "♙",
  k: "♚", q: "♛", r: "♜", b: "♝", n: "♞", p: "♟",
};
const FILES = "abcdefgh";

const state = {
  games: [],     // every game known to the page
  game: null,    // the game shown on the board
  selected: null, // square the human picked, e.g. "e2"
  pending: null, // id of the game with a move request in flight
};

const $ = (id) => document.getElementById(id);

function gamePath(id, suffix = "") {
  return `/api/games/${encodeURIComponent(id)}${suffix}`;
}

async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  let data = null;
  try {
    data = await res.json();
  } catch (_) {
    data = null;
  }
  if (!res.ok) {
    const msg = data && data.error ? data.error : `request failed (${res.status})`;
    throw new Error(msg);
  }
  return data;
}

function showError(err) {
  const box = $("message");
  box.textContent = err && err.message ? err.message : String(err);
  box.hidden = false;
}

function clearError() {
  const box = $("message");
  box.textContent = "";
  box.hidden = true;
}

// applyGame stores a game returned by the server and shows it if it is the
// open game. A reply for a game the user has since left only updates the list.
function applyGame(game) {
  const i = state.games.findIndex((g) => g.id === game.id);
  if (i >= 0) {
    state.games[i] = game;
  } else {
    state.games.unshift(game);
  }
  if (!state.game || state.game.id === game.id) {
    state.game = game;
    state.selected = null;
  }
  render();
}

async function loadGames() {
  try {
    state.games = await api("GET", "/api/games");
    render();
  } catch (err) {
    showError(err);
  }
}

async function openGame(id) {
  clearError();
  try {
    applyGame(await api("GET", gamePath(id)));
  } catch (err) {
    showError(err);
  }
}

async function refreshGame(id) {
  try {
    applyGame(await api("GET", gamePath(id)));
  } catch (err) {
    showError(err);
  }
}

// statusLine describes the game in one sentence.
function statusLine(game) {
  const how = game.method ? ` (${game.method})` : "";
  switch (game.status) {
    case "white_won":
      return `White won${how}.`;
    case "black_won":
      return `Black won${how}.`;
    case "draw":
      return `Draw${how}.`;
    default:
      return game.turn === game.human ? "Your move." : "Opponent to move.";
  }
}

function canPlay() {
  const game = state.game;
  return Boolean(game) &&
    game.status === "active" &&
    game.turn === game.human &&
    state.pending !== game.id;
}

function renderGames() {
  const list = $("games");
  list.replaceChildren();
  if (state.games.length === 0) {
    const li = document.createElement("li");
    li.className = "muted";
    li.textContent = "No games yet.";
    list.append(li);
    return;
  }
  for (const g of state.games) {
    const li = document.createElement("li");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.dataset.id = String(g.id);
    btn.textContent = `Game ${g.id}: ${g.human}, ${g.difficulty}. ${statusLine(g)}`;
    if (state.game && state.game.id === g.id) {
      btn.setAttribute("aria-current", "true");
    }
    li.append(btn);
    list.append(li);
  }
}

function renderInfo() {
  const game = state.game;
  if (!game) {
    $("game-info").textContent = "Pick a game, or start a new one.";
    return;
  }
  $("game-info").textContent =
    `Game ${game.id}: you play ${game.human} (${game.difficulty}). ${statusLine(game)}`;
  $("thinking").hidden = state.pending !== game.id;
  $("resign").disabled = game.status !== "active" || state.pending === game.id;
}

// parseFen returns grid[rank][file] with rank 0 = rank 1 and file 0 = a.
// Empty squares are "".
function parseFen(fen) {
  const grid = Array.from({ length: 8 }, () => Array(8).fill(""));
  const rows = fen.split(" ")[0].split("/");
  rows.forEach((text, i) => {
    const rank = 7 - i;
    let file = 0;
    for (const ch of text) {
      if (ch >= "1" && ch <= "8") {
        file += Number(ch);
      } else {
        grid[rank][file] = ch;
        file += 1;
      }
    }
  });
  return grid;
}

// lastMove returns the squares of the most recent move, or null.
function lastMove(game) {
  if (!game.moves || game.moves.length === 0) return null;
  const uci = game.moves[game.moves.length - 1].uci;
  if (!uci || uci.length < 4) return null;
  return { from: uci.slice(0, 2), to: uci.slice(2, 4) };
}

// targetsFor lists the squares the selected piece may move to.
function targetsFor(from) {
  if (!from || !canPlay()) return new Set();
  const targets = new Set();
  for (const m of state.game.legal) {
    if (m.startsWith(from)) targets.add(m.slice(2, 4));
  }
  return targets;
}

// moveFor returns the UCI move from one square to another, or null when the
// move is not legal. A promotion is sent as a queen move.
function moveFor(from, to) {
  const legal = state.game.legal;
  if (legal.includes(from + to)) return from + to;
  if (legal.includes(from + to + "q")) return from + to + "q";
  return null;
}

function renderBoard() {
  const board = $("board");
  board.replaceChildren();
  const game = state.game;
  if (!game) return;

  const grid = parseFen(game.fen);
  const flip = game.human === "black";
  const targets = targetsFor(state.selected);
  const last = lastMove(game);
  board.classList.toggle("inactive", !canPlay());

  for (let row = 0; row < 8; row++) {
    for (let col = 0; col < 8; col++) {
      const file = flip ? 7 - col : col;
      const rank = flip ? row : 7 - row;
      const name = FILES[file] + (rank + 1);
      const piece = grid[rank][file];

      const sq = document.createElement("button");
      sq.type = "button";
      sq.className = "sq " + ((file + rank) % 2 === 0 ? "dark" : "light");
      sq.dataset.square = name;
      sq.setAttribute("aria-label", piece ? `${name}, ${piece}` : name);
      if (piece) {
        const isBlack = piece === piece.toLowerCase() && piece !== piece.toUpperCase();
        sq.classList.add(isBlack ? "piece-black" : "piece-white");
      }
      sq.textContent = piece ? PIECES[piece] : "";
      if (name === state.selected) sq.classList.add("selected");
      if (targets.has(name)) sq.classList.add("target");
      if (last && (name === last.from || name === last.to)) sq.classList.add("last");
      board.append(sq);
    }
  }
}

function formatValue(v) {
  if (v === null || v === undefined || v === "") return "none";
  if (Array.isArray(v)) return v.length ? v.map(formatValue).join(", ") : "none";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

// turnDetails lists every field the server reported for the opponent's turn.
function turnDetails(turn) {
  const dl = document.createElement("dl");
  for (const [key, value] of Object.entries(turn)) {
    const dt = document.createElement("dt");
    dt.textContent = key;
    const dd = document.createElement("dd");
    dd.textContent = formatValue(value);
    dl.append(dt, dd);
  }
  return dl;
}

function renderMoves() {
  const ol = $("moves");
  ol.replaceChildren();
  const game = state.game;
  if (!game || !game.moves) return;

  game.moves.forEach((m, i) => {
    const li = document.createElement("li");
    const num = Math.floor(i / 2) + 1;
    const label = document.createElement("span");
    label.className = "san";
    label.textContent = `${i % 2 === 0 ? num + "." : num + "..."} ${m.san}`;
    li.append(label);
    if (m.by) {
      const by = document.createElement("span");
      by.className = "muted";
      by.textContent = ` (${m.by})`;
      li.append(by);
    }
    if (m.turn) {
      const details = document.createElement("details");
      const summary = document.createElement("summary");
      summary.textContent = "Opponent's decision";
      details.append(summary, turnDetails(m.turn));
      li.append(details);
    }
    ol.append(li);
  });
}

function render() {
  renderGames();
  renderInfo();
  renderBoard();
  renderMoves();
}

async function playMove(uci) {
  const id = state.game.id;
  clearError();
  state.pending = id;
  state.selected = null;
  render();
  try {
    applyGame(await api("POST", gamePath(id, "/moves"), { move: uci }));
  } catch (err) {
    showError(err);
    await refreshGame(id);
  } finally {
    if (state.pending === id) state.pending = null;
    render();
  }
}

$("new-game").addEventListener("submit", async (e) => {
  e.preventDefault();
  clearError();
  const data = new FormData(e.currentTarget);
  try {
    applyGame(await api("POST", "/api/games", {
      human: data.get("human"),
      difficulty: data.get("difficulty"),
    }));
    await loadGames();
  } catch (err) {
    showError(err);
  }
});

$("games").addEventListener("click", (e) => {
  const btn = e.target.closest("button[data-id]");
  if (btn) openGame(btn.dataset.id);
});

$("board").addEventListener("click", (e) => {
  const sq = e.target.closest("button[data-square]");
  if (!sq || !canPlay()) return;
  const name = sq.dataset.square;

  if (state.selected) {
    if (name === state.selected) {
      state.selected = null;
      render();
      return;
    }
    const uci = moveFor(state.selected, name);
    if (uci) {
      playMove(uci);
      return;
    }
  }

  // Pick a piece that has at least one legal move.
  const movable = state.game.legal.some((m) => m.startsWith(name));
  state.selected = movable ? name : null;
  render();
});

$("resign").addEventListener("click", async () => {
  const game = state.game;
  if (!game || game.status !== "active" || state.pending === game.id) return;
  if (!window.confirm("Resign this game?")) return;
  clearError();
  try {
    applyGame(await api("POST", gamePath(game.id, "/resign")));
  } catch (err) {
    showError(err);
  }
});

loadGames();
