// llmchat page: a sidebar of conversations, a streamed message log, a
// composer and a model picker. It talks only to the /api JSON API.
(function () {
  "use strict";

  const $ = (id) => document.getElementById(id);
  const listEl = $("conversations");
  const logEl = $("log");
  const form = $("composer");
  const promptEl = $("prompt");
  const modelEl = $("model");
  const sendBtn = $("send");
  const stopBtn = $("stop");
  const newChatBtn = $("new-chat");
  const deleteBtn = $("delete");
  const titleEl = $("title");
  const workspaceEl = $("workspace");
  const setWorkspaceBtn = $("set-workspace");

  let conversations = [];
  let current = null; // the open conversation, or null
  let busy = false; // a message is streaming
  let activeId = null; // the conversation whose turn is streaming, or null
  let stick = true; // keep the log scrolled to the bottom

  // renderMarkdown escapes the text, then turns fenced blocks and inline
  // code into tags. Nothing else produces markup.
  function renderMarkdown(text) {
    const escaped = String(text)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
    const lines = escaped.split("\n");
    const out = [];
    let i = 0;
    while (i < lines.length) {
      if (lines[i].startsWith("```")) {
        const code = [];
        i++;
        while (i < lines.length && lines[i].trim() !== "```") {
          code.push(lines[i]);
          i++;
        }
        i++; // skip the closing fence
        out.push("<pre><code>" + code.join("\n") + "</code></pre>");
      } else {
        out.push(lines[i].replace(/`([^`]*)`/g, "<code>$1</code>"));
        i++;
      }
    }
    return out.join("\n");
  }

  // request calls the JSON API and returns the parsed body, or null for 204.
  // A failed call throws an Error reading "Error STATUS: BODY".
  async function request(method, url, body) {
    const opts = { method: method, headers: {} };
    if (body !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    const resp = await fetch(url, opts);
    if (!resp.ok) {
      throw new Error("Error " + resp.status + ": " + (await resp.text()));
    }
    if (resp.status === 204) return null;
    return resp.json();
  }

  function conversationURL(id) {
    return "/api/conversations/" + encodeURIComponent(id);
  }

  function metaText(info) {
    if (!info || !info.backend) return "";
    let s = info.route ? "route " + info.route + " · " : "";
    return s + info.backend + "/" + (info.model || "?");
  }

  function keepScrolled(force) {
    if (force || stick) logEl.scrollTop = logEl.scrollHeight;
  }

  logEl.addEventListener("scroll", function () {
    stick = logEl.scrollHeight - logEl.scrollTop - logEl.clientHeight < 24;
  });

  function showError(text) {
    const div = document.createElement("div");
    div.className = "msg err";
    div.textContent = text;
    logEl.appendChild(div);
    keepScrolled();
  }

  // newAssistant builds an assistant bubble with its content and meta line.
  // The reasoning block is created on demand, before the content.
  function newAssistant() {
    const el = document.createElement("div");
    el.className = "msg assistant";
    const content = document.createElement("div");
    content.className = "content";
    const meta = document.createElement("div");
    meta.className = "meta";
    el.append(content, meta);
    let reasoning = null;
    return {
      el: el,
      content: content,
      setReasoning: function (text) {
        if (!reasoning) {
          const details = document.createElement("details");
          details.className = "reasoning";
          const summary = document.createElement("summary");
          summary.textContent = "Reasoning";
          reasoning = document.createElement("div");
          details.append(summary, reasoning);
          el.insertBefore(details, content);
        }
        reasoning.textContent = text;
      },
      setContent: function (text) {
        content.innerHTML = renderMarkdown(text);
      },
      setMeta: function (text) {
        if (text) meta.textContent = text;
        else meta.remove();
      }
    };
  }

  function userBubble(text) {
    const el = document.createElement("div");
    el.className = "msg user";
    const content = document.createElement("div");
    content.className = "content";
    content.textContent = text;
    el.appendChild(content);
    return el;
  }

  // prettyArgs shows a tool call's JSON arguments indented, or the raw text
  // when it is not JSON.
  function prettyArgs(raw) {
    const s = raw == null ? "" : String(raw);
    try {
      return JSON.stringify(JSON.parse(s), null, 2);
    } catch (err) {
      return s;
    }
  }

  // addTool appends a collapsible block for one tool call to the log. Its
  // result is filled in later by setResult.
  function addTool(callID, name, rawArgs) {
    const details = document.createElement("details");
    details.className = "tool";
    details.dataset.call = callID;
    details.open = true;
    const summary = document.createElement("summary");
    summary.textContent = "\u{1F527} " + (name || "tool");
    const args = document.createElement("pre");
    args.className = "args";
    args.textContent = prettyArgs(rawArgs);
    const result = document.createElement("pre");
    result.className = "result";
    details.append(summary, args, result);
    logEl.appendChild(details);
    keepScrolled();
    return details;
  }

  // ensureTool returns the block for a call, creating it when the event for
  // the call was not seen.
  function ensureTool(calls, ev) {
    const id = String(ev.call_id || "");
    if (!calls[id]) calls[id] = addTool(id, ev.name, ev.arguments);
    return calls[id];
  }

  function setResult(block, text) {
    block.querySelector("pre.result").textContent = String(text == null ? "" : text);
    const pending = block.querySelector(".approval");
    if (pending) pending.remove();
  }

  // approvalQuestion words the question for a call that needs approval. A
  // bash command and a file change show what they will do.
  function approvalQuestion(name, rawArgs) {
    let args = {};
    try {
      args = JSON.parse(String(rawArgs || "")) || {};
    } catch (err) {
      args = {};
    }
    if (name === "bash" && args.command != null) return "Run command: " + args.command;
    if ((name === "write_file" || name === "edit_file") && args.path != null) {
      return "Change " + args.path + "?";
    }
    return "Run " + (name || "tool") + "?";
  }

  // addApproval puts Approve and Deny buttons in a call's block. The answer
  // goes to the approvals endpoint; a failed answer shows its error and
  // re-enables the buttons.
  function addApproval(block, conversationID, ev) {
    const box = document.createElement("div");
    box.className = "approval";
    const approve = document.createElement("button");
    approve.type = "button";
    approve.className = "approve";
    approve.textContent = "Approve";
    const deny = document.createElement("button");
    deny.type = "button";
    deny.className = "deny";
    deny.textContent = "Deny";
    box.append(approvalQuestion(ev.name, ev.arguments) + " ", approve, deny);
    block.appendChild(box);
    keepScrolled();

    async function decide(ok) {
      approve.disabled = true;
      deny.disabled = true;
      try {
        await request(
          "POST",
          conversationURL(conversationID) + "/approvals/" + encodeURIComponent(String(ev.call_id || "")),
          { approve: ok }
        );
        box.classList.add("done");
        box.textContent = ok ? "Approved" : "Denied";
      } catch (err) {
        approve.disabled = false;
        deny.disabled = false;
        showError(err.message);
      }
    }
    approve.addEventListener("click", function () {
      decide(true);
    });
    deny.addEventListener("click", function () {
      decide(false);
    });
  }

  // appendMessage shows one stored message at the end of the log. A tool
  // call's block follows its message; a tool result fills that block.
  function appendMessage(m, calls) {
    if (m.role === "user") {
      logEl.appendChild(userBubble(m.content));
      return;
    }
    if (m.role === "assistant") {
      if (m.content) {
        const bubble = newAssistant();
        if (m.reasoning) bubble.setReasoning(m.reasoning);
        bubble.setContent(m.content);
        bubble.setMeta(metaText({ route: m.route, backend: m.backend, model: m.model }));
        logEl.appendChild(bubble.el);
      }
      for (const c of m.tool_calls || []) {
        calls[c.id] = addTool(c.id, c.name, c.arguments);
      }
      return;
    }
    if (m.role === "tool") {
      const block = calls[m.tool_call_id];
      if (block) setResult(block, m.content);
    }
  }

  // showWorkspace fills the workspace field from a conversation.
  function showWorkspace(conv) {
    workspaceEl.value = conv.workspace || "";
  }

  function renderSidebar() {
    listEl.textContent = "";
    for (const c of conversations) {
      const li = document.createElement("li");
      li.textContent = c.title || "New chat";
      if (current && current.id === c.id) li.classList.add("active");
      li.addEventListener("click", function () {
        openConversation(c.id);
      });
      listEl.appendChild(li);
    }
  }

  async function refreshSidebar() {
    const list = await request("GET", "/api/conversations");
    conversations = Array.isArray(list) ? list : [];
    renderSidebar();
    if (current) {
      const match = conversations.find((c) => c.id === current.id);
      if (match) titleEl.textContent = match.title || "New chat";
    }
  }

  function replaceConversation(conv) {
    const i = conversations.findIndex((c) => c.id === conv.id);
    if (i >= 0) conversations[i] = conv;
    if (current && current.id === conv.id) current = conv;
  }

  function addOption(name) {
    const opt = document.createElement("option");
    opt.value = name;
    opt.textContent = name;
    modelEl.appendChild(opt);
  }

  function ensureModelOption(name) {
    if (!name) return;
    const exists = Array.from(modelEl.options).some((o) => o.value === name);
    if (!exists) addOption(name);
  }

  async function loadRoutes() {
    const names = ["auto"];
    try {
      const routes = await request("GET", "/api/routes");
      if (Array.isArray(routes)) {
        for (const r of routes) {
          if (r !== "auto") names.push(r);
        }
      }
    } catch (err) {
      // Keep only "auto".
    }
    modelEl.textContent = "";
    for (const name of names) addOption(name);
  }

  async function openConversation(id) {
    let data;
    try {
      data = await request("GET", conversationURL(id));
    } catch (err) {
      showError(err.message);
      return;
    }
    current = data.conversation;
    titleEl.textContent = current.title || "New chat";
    ensureModelOption(current.model);
    modelEl.value = current.model;
    showWorkspace(current);
    logEl.textContent = "";
    const calls = {};
    for (const m of data.messages || []) appendMessage(m, calls);
    renderSidebar();
    stick = true;
    keepScrolled(true);
  }

  function setBusy(on) {
    busy = on;
    promptEl.disabled = on;
    sendBtn.disabled = on;
    stopBtn.disabled = !on;
  }

  // stopApprovals ends the approval prompts still waiting when a turn stops:
  // the turn no longer exists to receive an answer.
  function stopApprovals() {
    for (const box of logEl.querySelectorAll(".approval:not(.done)")) {
      box.classList.add("done");
      box.textContent = "Stopped";
    }
  }

  // sendMessage posts the prompt to the open conversation and streams the
  // answer into a new assistant bubble.
  async function sendMessage(text) {
    setBusy(true);
    try {
      if (!current) {
        const conv = await request("POST", "/api/conversations", {
          model: modelEl.value,
          workspace: workspaceEl.value.trim()
        });
        conversations.unshift(conv);
        current = conv;
        titleEl.textContent = "New chat";
        showWorkspace(conv);
        renderSidebar();
      }
    } catch (err) {
      showError(err.message);
      setBusy(false);
      return;
    }

    const id = current.id;
    activeId = id;
    stick = true;
    logEl.appendChild(userBubble(text));
    promptEl.value = "";
    let bubble = newAssistant();
    logEl.appendChild(bubble.el);
    keepScrolled(true);

    // Each model step starts with an info event. The first one uses the
    // bubble above; each later one closes the current bubble and opens a
    // new one after the tool blocks.
    let started = false;
    let bubbleInfo = null; // the info that started the current bubble
    let content = "";
    let reasoning = "";
    let finished = false;
    const calls = {}; // call id -> tool block

    function startBubble() {
      bubble.setMeta(metaText(bubbleInfo));
      bubble = newAssistant();
      logEl.appendChild(bubble.el);
      content = "";
      reasoning = "";
    }

    function handleLine(line) {
      if (line.endsWith("\r")) line = line.slice(0, -1);
      if (!line.startsWith("data: ")) return;
      let ev;
      try {
        ev = JSON.parse(line.slice(6));
      } catch (err) {
        return;
      }
      if (!ev || typeof ev !== "object") return;
      switch (ev.type) {
        case "info":
          if (started) startBubble();
          started = true;
          bubbleInfo = { route: ev.route || "", backend: ev.backend || "", model: ev.model || "" };
          break;
        case "content":
          content += String(ev.text == null ? "" : ev.text);
          bubble.setContent(content);
          keepScrolled();
          break;
        case "reasoning":
          reasoning += String(ev.text == null ? "" : ev.text);
          bubble.setReasoning(reasoning);
          keepScrolled();
          break;
        case "tool_call":
          calls[String(ev.call_id || "")] = addTool(String(ev.call_id || ""), ev.name, ev.arguments);
          break;
        case "approval":
          addApproval(ensureTool(calls, ev), id, ev);
          break;
        case "tool_result":
          setResult(ensureTool(calls, ev), ev.text);
          break;
        case "error":
          showError(String(ev.error || "unknown error"));
          break;
        case "stopped":
          stopApprovals();
          stopNote();
          break;
        case "done":
          finished = true;
          break;
        default:
          break;
      }
    }

    try {
      const resp = await fetch(conversationURL(id) + "/messages", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text: text })
      });
      if (!resp.ok) {
        const detail = await resp.text();
        bubble.el.classList.add("err");
        bubble.content.textContent = "Error " + resp.status + ": " + detail;
        return;
      }

      const reader = resp.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      while (!finished) {
        const chunk = await reader.read();
        if (chunk.done) break;
        buffer += decoder.decode(chunk.value, { stream: true });
        let nl = buffer.indexOf("\n");
        while (!finished && nl >= 0) {
          handleLine(buffer.slice(0, nl));
          buffer = buffer.slice(nl + 1);
          nl = buffer.indexOf("\n");
        }
      }
      if (finished) {
        reader.cancel().catch(function () {});
      } else {
        buffer += decoder.decode();
        if (buffer) handleLine(buffer);
      }
    } catch (err) {
      bubble.el.classList.add("err");
      bubble.content.textContent = "Error: " + err.message;
    } finally {
      bubble.setMeta(metaText(bubbleInfo));
      activeId = null;
      setBusy(false);
      promptEl.focus();
      keepScrolled(true);
      try {
        await refreshSidebar();
      } catch (err) {
        showError(err.message);
      }
    }
  }

  // stopNote writes the note that a turn was stopped at the end of the log.
  function stopNote() {
    const div = document.createElement("div");
    div.className = "msg note";
    div.textContent = "Turn stopped";
    logEl.appendChild(div);
    keepScrolled();
  }

  // Stop asks the server to end the running turn. The server ends it and
  // sends a stopped event on the open stream, so the page only sends the
  // request. A 404 means the turn had already finished, which is not an error.
  stopBtn.addEventListener("click", async function () {
    if (!activeId) return;
    stopBtn.disabled = true;
    try {
      await request("POST", conversationURL(activeId) + "/stop");
    } catch (err) {
      if (!String(err.message).startsWith("Error 404")) {
        showError(err.message);
        if (busy) stopBtn.disabled = false;
      }
    }
  });

  form.addEventListener("submit", function (e) {
    e.preventDefault();
    if (busy) return;
    const text = promptEl.value;
    if (text.trim() === "") return;
    sendMessage(text);
  });

  promptEl.addEventListener("keydown", function (e) {
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      form.requestSubmit();
    }
  });

  newChatBtn.addEventListener("click", async function () {
    try {
      const conv = await request("POST", "/api/conversations", {
        model: modelEl.value,
        workspace: workspaceEl.value.trim()
      });
      conversations.unshift(conv);
      renderSidebar();
      await openConversation(conv.id);
    } catch (err) {
      showError(err.message);
    }
  });

  modelEl.addEventListener("change", async function () {
    if (!current) return;
    const id = current.id;
    try {
      const conv = await request("PATCH", conversationURL(id), { model: modelEl.value });
      replaceConversation(conv);
    } catch (err) {
      showError(err.message);
      if (current && current.id === id) modelEl.value = current.model;
    }
  });

  setWorkspaceBtn.addEventListener("click", async function () {
    if (!current) return;
    const id = current.id;
    try {
      const conv = await request("PATCH", conversationURL(id), { workspace: workspaceEl.value.trim() });
      replaceConversation(conv);
      if (current && current.id === id) showWorkspace(conv);
    } catch (err) {
      showError(err.message);
    }
  });

  deleteBtn.addEventListener("click", async function () {
    if (!current) return;
    if (!confirm("Delete this conversation?")) return;
    const id = current.id;
    try {
      await request("DELETE", conversationURL(id));
    } catch (err) {
      showError(err.message);
      return;
    }
    const i = conversations.findIndex((c) => c.id === id);
    if (i >= 0) conversations.splice(i, 1);
    current = null;
    if (conversations.length > 0) {
      const next = Math.max(0, Math.min(i, conversations.length - 1));
      await openConversation(conversations[next].id);
    } else {
      renderSidebar();
      logEl.textContent = "";
      titleEl.textContent = "";
      workspaceEl.value = "";
    }
  });

  async function init() {
    await loadRoutes();
    try {
      const list = await request("GET", "/api/conversations");
      conversations = Array.isArray(list) ? list : [];
    } catch (err) {
      showError(err.message);
    }
    renderSidebar();
    if (conversations.length > 0) await openConversation(conversations[0].id);
  }

  init();
})();
