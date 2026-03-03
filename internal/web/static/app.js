    const personaGroupEl = document.getElementById("personaGroup");
    const personaMetaEl = document.getElementById("personaMeta");
    const personaListEl = document.getElementById("personaList");
    const problemEl = document.getElementById("problem");
    const runBtn = document.getElementById("runBtn");
    const stopBtn = document.getElementById("stopBtn");
    const statusText = document.getElementById("statusText");
    const errorText = document.getElementById("errorText");
    const progressWrapEl = document.getElementById("progressWrap");
    const debateWindowEl = document.getElementById("debateWindow");
    const turnMetaEl = document.getElementById("turnMeta");
    const audienceModeEl = document.getElementById("audienceMode");
    const maxTurnsEl = document.getElementById("maxTurns");
    const consensusThresholdEl = document.getElementById("consensusThreshold");
    const maxNoProgressJudgesEl = document.getElementById("maxNoProgressJudges");
    const noProgressEpsilonEl = document.getElementById("noProgressEpsilon");
    const unlimitedHardMaxTurnsEl = document.getElementById("unlimitedHardMaxTurns");
    const directHandoffJudgeEveryEl = document.getElementById("directHandoffJudgeEvery");
    const llmHistoryTurnWindowEl = document.getElementById("llmHistoryTurnWindow");
    const maxDurationSecondsEl = document.getElementById("maxDurationSeconds");
    const maxTotalTokensEl = document.getElementById("maxTotalTokens");
    const runTimeoutSecondsEl = document.getElementById("runTimeoutSeconds");
    const advancedResetBtn = document.getElementById("advancedResetBtn");

    const predefinedGroups = [
      { label: "아이디어", path: "./exmaples/personas.ideas.json" },
      { label: "브레인스토밍", path: "./exmaples/personas.brainstorming.json" },
      { label: "PM", path: "./exmaples/personas.pm.json" },
      { label: "컴퍼니", path: "./exmaples/personas.company.json" },
      { label: "SEC", path: "./exmaples/personas.sec.json" },
      { label: "친구", path: "./exmaples/personas.friend.json" },
      { label: "뮤직", path: "./exmaples/personas.music.json" }
    ];

    let personaGroups = [];
    let selectedPersonaPath = "";
	    let currentRunID = "";
	    let currentStream = null;
	    let turnCount = 0;
	    let stopRequested = false;
	    let latestPersonaLoadSeq = 0;
	    const maxRenderedTurnCards = 320;

    function closeCurrentStream() {
      if (!currentStream) return;
      currentStream.close();
      currentStream = null;
    }

    function setDebateRunning(isRunning) {
      runBtn.disabled = isRunning;
      stopBtn.disabled = !isRunning;
    }

    function setTurnMeta(count, state) {
      let text = String(count) + " 턴";
      if (state) {
        text += " · " + state;
      }
      turnMetaEl.textContent = text;
    }

    function normalizeKey(value) {
      return String(value || "").trim().toLowerCase();
    }

    function clearActivePersona() {
      const activeItems = personaListEl.querySelectorAll(".persona-item.is-active");
      activeItems.forEach((item) => item.classList.remove("is-active"));
    }

    function highlightSpeakerPersona(speakerID, speakerName) {
      const idKey = normalizeKey(speakerID);
      const nameKey = normalizeKey(speakerName);
      const items = personaListEl.querySelectorAll(".persona-item");
      let matched = null;

      items.forEach((item) => {
        item.classList.remove("is-active");
      });

      if (idKey) {
        matched = Array.from(items).find((item) => normalizeKey(item.dataset.personaId) === idKey) || null;
      }
      if (!matched && nameKey) {
        matched = Array.from(items).find((item) => normalizeKey(item.dataset.personaName) === nameKey) || null;
      }
      if (matched) {
        matched.classList.add("is-active");
      }
    }

    function showProgress(text) {
      progressWrapEl.classList.add("active");
      progressWrapEl.setAttribute("aria-hidden", "false");
    }

    function hideProgress() {
      progressWrapEl.classList.remove("active");
      progressWrapEl.setAttribute("aria-hidden", "true");
    }

    // Auto-resize textarea
    problemEl.addEventListener("input", function() {
      this.style.height = "auto";
      this.style.height = (this.scrollHeight) + "px";
    });

    function hashText(text) {
      const value = String(text || "");
      let hash = 0;
      for (let i = 0; i < value.length; i += 1) {
        hash = (hash * 31 + value.charCodeAt(i)) % 2147483647;
      }
      return hash;
    }

    function hueFromText(text, offset) {
      const base = (hashText(text) + Number(offset || 0)) % 140;
      return 165 + base;
    }

    function initialsFromText(text) {
      const cleaned = String(text || "").trim();
      if (!cleaned) {
        return "?";
      }
      const parts = cleaned.split(/\s+/).filter(Boolean);
      if (parts.length === 1) {
        return parts[0].slice(0, 2).toUpperCase();
      }
      return (parts[0].charAt(0) + parts[1].charAt(0)).toUpperCase();
    }

    function parseJSON(text) {
      try {
        return JSON.parse(text);
      } catch (_) {
        return null;
      }
    }

    function sanitizeTurnContent(content) {
      const lines = String(content || "").split("\n");
      const visible = [];
      for (const line of lines) {
        const cleaned = stripEvidenceMeta(String(line || ""));
        const trimmed = cleaned.trim();
        if (!trimmed) {
          continue;
        }
        if (isListMarkerOnly(trimmed)) {
          continue;
        }
        if (isHiddenDirectiveLine(trimmed)) {
          continue;
        }
        visible.push(trimmed);
      }
      return visible.join("\n").trim();
    }

    function escapeHTML(value) {
      return String(value || "")
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;");
    }

    function escapeAttr(value) {
      return String(value || "")
        .replace(/&/g, "&amp;")
        .replace(/"/g, "&quot;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;");
    }

    function sanitizeMarkdownURL(rawURL) {
      const value = String(rawURL || "").trim();
      if (!value) {
        return "";
      }

      const lower = value.toLowerCase();
      if (lower.startsWith("mailto:")) {
        return value;
      }
      if (!lower.startsWith("http://") && !lower.startsWith("https://")) {
        return "";
      }

      try {
        const parsed = new URL(value);
        if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
          return "";
        }
        return parsed.href;
      } catch (_) {
        return "";
      }
    }

    function renderInlineMarkdown(text) {
      let value = String(text || "");
      const tokens = [];
      const tokenKey = (index) => "\u0000MDTOK" + String(index) + "\u0000";
      const putToken = (html) => {
        const idx = tokens.length;
        tokens.push(html);
        return tokenKey(idx);
      };

      value = value.replace(/`([^`\n]+)`/g, function (_, code) {
        return putToken("<code>" + escapeHTML(code) + "</code>");
      });

      value = value.replace(/\[([^\]\n]+)\]\(([^)\n]+)\)/g, function (_, label, url) {
        const safeURL = sanitizeMarkdownURL(url);
        if (!safeURL) {
          return putToken(escapeHTML(label) + " (" + escapeHTML(url) + ")");
        }
        return putToken(
          '<a href="' + escapeAttr(safeURL) + '" target="_blank" rel="noopener noreferrer">' +
            escapeHTML(label) +
          "</a>"
        );
      });

      let html = escapeHTML(value);
      html = html.replace(/\*\*([^*\n]+)\*\*/g, "<strong>$1</strong>");
      html = html.replace(/__([^_\n]+)__/g, "<strong>$1</strong>");
      html = html.replace(/(^|[^\*])\*([^*\n]+)\*(?!\*)/g, "$1<em>$2</em>");
      html = html.replace(/(^|[^_])_([^_\n]+)_(?!_)/g, "$1<em>$2</em>");
      html = html.replace(/~~([^~\n]+)~~/g, "<del>$1</del>");
      html = html.replace(/\u0000MDTOK(\d+)\u0000/g, function (_, idx) {
        const i = Number(idx);
        if (Number.isNaN(i) || !tokens[i]) {
          return "";
        }
        return tokens[i];
      });
      return html;
    }

    function isMarkdownBlockBoundary(line) {
      const text = String(line || "");
      const trimmed = text.trim();
      if (!trimmed) {
        return true;
      }
      return /^```/.test(trimmed) ||
        /^\s{0,3}(#{1,6})\s+/.test(text) ||
        /^\s{0,3}>\s?/.test(text) ||
        /^\s{0,3}[-*+]\s+/.test(text) ||
        /^\s{0,3}\d+\.\s+/.test(text) ||
        /^\s{0,3}(?:-{3,}|\*{3,}|_{3,})\s*$/.test(text);
    }

    function markdownToHTML(content) {
      const source = String(content || "").replace(/\r\n/g, "\n").trim();
      if (!source) {
        return "";
      }

      const lines = source.split("\n");
      const out = [];
      let i = 0;

      while (i < lines.length) {
        const line = lines[i];
        const trimmed = line.trim();

        if (!trimmed) {
          i += 1;
          continue;
        }

        const fenceMatch = trimmed.match(/^```([A-Za-z0-9_+-]+)?\s*$/);
        if (fenceMatch) {
          const lang = (fenceMatch[1] || "").toLowerCase();
          i += 1;
          const codeLines = [];
          while (i < lines.length && !/^```/.test(lines[i].trim())) {
            codeLines.push(lines[i]);
            i += 1;
          }
          if (i < lines.length) {
            i += 1;
          }
          const classAttr = lang ? ' class="language-' + escapeAttr(lang) + '"' : "";
          out.push("<pre><code" + classAttr + ">" + escapeHTML(codeLines.join("\n")) + "</code></pre>");
          continue;
        }

        const headingMatch = line.match(/^\s{0,3}(#{1,6})\s+(.+)$/);
        if (headingMatch) {
          const level = headingMatch[1].length;
          out.push("<h" + String(level) + ">" + renderInlineMarkdown(headingMatch[2].trim()) + "</h" + String(level) + ">");
          i += 1;
          continue;
        }

        if (/^\s{0,3}(?:-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
          out.push("<hr />");
          i += 1;
          continue;
        }

        if (/^\s{0,3}>\s?/.test(line)) {
          const quoteLines = [];
          while (i < lines.length) {
            const quoteMatch = lines[i].match(/^\s{0,3}>\s?(.*)$/);
            if (!quoteMatch) {
              break;
            }
            quoteLines.push(quoteMatch[1]);
            i += 1;
          }
          out.push("<blockquote>" + markdownToHTML(quoteLines.join("\n")) + "</blockquote>");
          continue;
        }

        if (/^\s{0,3}[-*+]\s+/.test(line)) {
          const items = [];
          while (i < lines.length) {
            const itemMatch = lines[i].match(/^\s{0,3}[-*+]\s+(.+)$/);
            if (!itemMatch) {
              break;
            }
            items.push("<li>" + renderInlineMarkdown(itemMatch[1].trim()) + "</li>");
            i += 1;
          }
          out.push("<ul>" + items.join("") + "</ul>");
          continue;
        }

        if (/^\s{0,3}\d+\.\s+/.test(line)) {
          const items = [];
          while (i < lines.length) {
            const itemMatch = lines[i].match(/^\s{0,3}\d+\.\s+(.+)$/);
            if (!itemMatch) {
              break;
            }
            items.push("<li>" + renderInlineMarkdown(itemMatch[1].trim()) + "</li>");
            i += 1;
          }
          out.push("<ol>" + items.join("") + "</ol>");
          continue;
        }

        const paragraph = [];
        while (i < lines.length) {
          const current = lines[i];
          if (!String(current || "").trim()) {
            break;
          }
          if (paragraph.length > 0 && isMarkdownBlockBoundary(current)) {
            break;
          }
          paragraph.push(String(current || "").trim());
          i += 1;
        }
        out.push("<p>" + paragraph.map(renderInlineMarkdown).join("<br />") + "</p>");
      }

      return out.join("");
    }

    function stripEvidenceMeta(line) {
      return String(line || "").replace(
        /\(?\s*(?:evidence_type\s*=\s*)?[^,\)\s]+(?:\s*,\s*|\s+)\s*confidence\s*=\s*[^,\)\s]+\s*\)?[.!?。．…]*/gi,
        ""
      );
    }

    function isListMarkerOnly(line) {
      if (line === "-" || line === "*" || line === "+") {
        return true;
      }
      return /^[0-9]+\.$/.test(line);
    }

    function isHiddenDirectiveLine(line) {
      const normalizedRaw = normalizeDirectiveCandidate(line);
      if (/^\(?\s*(?:evidence_type\s*=\s*)?[^,\)\s]+(?:\s*,\s*|\s+)\s*confidence\s*=\s*[^,\)\s]+\s*\)?[.!?。．…]*$/i.test(normalizedRaw)) {
        return true;
      }
      const normalized = normalizedRaw.toLowerCase();
      return hasDirectivePrefix(normalized, "handoff_ask") ||
        hasDirectivePrefix(normalized, "next") ||
        hasDirectivePrefix(normalized, "close") ||
        hasDirectivePrefix(normalized, "new_point") ||
        hasDirectivePrefix(normalized, "new-point") ||
        hasDirectivePrefix(normalized, "issue_update") ||
        hasDirectivePrefix(normalized, "persuasion_update") ||
        hasDirectivePrefix(normalized, "meta_delta") ||
        hasDirectivePrefix(normalized, "self_check") ||
        hasDirectivePrefix(normalized, "option_a") ||
        hasDirectivePrefix(normalized, "option_b") ||
        hasDirectivePrefix(normalized, "scorecard") ||
        hasDirectivePrefix(normalized, "scorecard_reason");
    }

    function hasDirectivePrefix(line, key) {
      if (!line.startsWith(key)) {
        return false;
      }
      const rest = line.slice(key.length).trimStart();
      return rest.startsWith(":") || rest.startsWith("=") || rest.startsWith("：");
    }

    function normalizeDirectiveCandidate(line) {
      let value = String(line || "").trim();
      while (value) {
        const prev = value;
        value = value.trim();

        if (value.startsWith(">")) {
          value = value.slice(1).trim();
        }

        const lower = value.toLowerCase();
        if (lower.startsWith("- [ ] ") || lower.startsWith("- [x] ")) {
          value = value.slice(6).trim();
        } else if (/^[-*+]\s+/.test(value)) {
          value = value.replace(/^[-*+]\s+/, "").trim();
        } else {
          const match = value.match(/^\d+\.\s+/);
          if (match) {
            value = value.slice(match[0].length).trim();
          }
        }

        if (value === prev) {
          break;
        }
      }
      return value;
    }

    function parseOptionalIntInput(el, fieldName, minValue) {
      const raw = String((el && el.value) || "").trim();
      if (!raw) {
        return undefined;
      }
      if (!/^-?\d+$/.test(raw)) {
        throw new Error(fieldName + " 값은 정수여야 합니다.");
      }
      const parsed = Number(raw);
      if (!Number.isSafeInteger(parsed)) {
        throw new Error(fieldName + " 값이 너무 큽니다.");
      }
      if (typeof minValue === "number" && parsed < minValue) {
        throw new Error(fieldName + " 값은 " + String(minValue) + " 이상이어야 합니다.");
      }
      return parsed;
    }

    function parseOptionalFloatInput(el, fieldName, minValue, maxValue, exclusiveMin) {
      const raw = String((el && el.value) || "").trim();
      if (!raw) {
        return undefined;
      }
      const parsed = Number(raw);
      if (!Number.isFinite(parsed)) {
        throw new Error(fieldName + " 값은 숫자여야 합니다.");
      }
      if (exclusiveMin === true) {
        if (parsed <= minValue) {
          throw new Error(fieldName + " 값은 " + String(minValue) + "보다 커야 합니다.");
        }
      } else if (typeof minValue === "number" && parsed < minValue) {
        throw new Error(fieldName + " 값은 " + String(minValue) + " 이상이어야 합니다.");
      }
      if (typeof maxValue === "number" && parsed > maxValue) {
        throw new Error(fieldName + " 값은 " + String(maxValue) + " 이하여야 합니다.");
      }
      return parsed;
    }

    function collectRuntimeOptions() {
      const options = {};
      const audienceMode = String((audienceModeEl && audienceModeEl.value) || "").trim();
      if (audienceMode) {
        options.audience_mode = audienceMode;
      }

      const maxTurns = parseOptionalIntInput(maxTurnsEl, "Max Turns", 0);
      if (typeof maxTurns === "number") {
        options.max_turns = maxTurns;
      }

      const consensusThreshold = parseOptionalFloatInput(consensusThresholdEl, "Consensus Threshold", 0, 1, false);
      if (typeof consensusThreshold === "number") {
        options.consensus_threshold = consensusThreshold;
      }

      const maxNoProgressJudges = parseOptionalIntInput(maxNoProgressJudgesEl, "Max No Progress Judges", 1);
      if (typeof maxNoProgressJudges === "number") {
        options.max_no_progress_judges = maxNoProgressJudges;
      }

      const noProgressEpsilon = parseOptionalFloatInput(noProgressEpsilonEl, "No Progress Epsilon", 0, undefined, true);
      if (typeof noProgressEpsilon === "number") {
        options.no_progress_epsilon = noProgressEpsilon;
      }

      const unlimitedHardMaxTurns = parseOptionalIntInput(unlimitedHardMaxTurnsEl, "Unlimited Hard Max Turns", 1);
      if (typeof unlimitedHardMaxTurns === "number") {
        options.unlimited_hard_max_turns = unlimitedHardMaxTurns;
      }

      const directHandoffJudgeEvery = parseOptionalIntInput(directHandoffJudgeEveryEl, "Direct Handoff Judge Every", 1);
      if (typeof directHandoffJudgeEvery === "number") {
        options.direct_handoff_judge_every = directHandoffJudgeEvery;
      }

      const llmHistoryTurnWindow = parseOptionalIntInput(llmHistoryTurnWindowEl, "LLM History Turn Window", 1);
      if (typeof llmHistoryTurnWindow === "number") {
        options.llm_history_turn_window = llmHistoryTurnWindow;
      }

      const maxDurationSeconds = parseOptionalIntInput(maxDurationSecondsEl, "Max Duration (sec)", 1);
      if (typeof maxDurationSeconds === "number") {
        options.max_duration_seconds = maxDurationSeconds;
      }

      const maxTotalTokens = parseOptionalIntInput(maxTotalTokensEl, "Max Total Tokens", 1);
      if (typeof maxTotalTokens === "number") {
        options.max_total_tokens = maxTotalTokens;
      }

      const runTimeoutSeconds = parseOptionalIntInput(runTimeoutSecondsEl, "Run Timeout (sec)", 1);
      if (typeof runTimeoutSeconds === "number") {
        options.run_timeout_seconds = runTimeoutSeconds;
      }

      return options;
    }

    function resetAdvancedOptions() {
      if (audienceModeEl) {
        audienceModeEl.value = "";
      }
      const inputs = [
        maxTurnsEl,
        consensusThresholdEl,
        maxNoProgressJudgesEl,
        noProgressEpsilonEl,
        unlimitedHardMaxTurnsEl,
        directHandoffJudgeEveryEl,
        llmHistoryTurnWindowEl,
        maxDurationSecondsEl,
        maxTotalTokensEl,
        runTimeoutSecondsEl
      ];
      inputs.forEach((el) => {
        if (el) {
          el.value = "";
        }
      });
    }

    async function fetchPersonas(path) {
      const url = path ? "/api/personas?path=" + encodeURIComponent(path) : "/api/personas";
      const res = await fetch(url);
      const payload = await res.json();
      if (!res.ok) throw new Error(payload.error || "persona 로딩 실패");
      return payload;
    }

    async function createDebateRun(problem) {
      const requestBody = { problem: problem };
      if (selectedPersonaPath) {
        requestBody.persona_path = selectedPersonaPath;
      }
      const runtimeOptions = collectRuntimeOptions();
      Object.assign(requestBody, runtimeOptions);

      const res = await fetch("/api/debate/stream/start", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(requestBody)
      });
      const payload = await res.json();
      if (!res.ok) {
        throw new Error(payload.error || "토론 시작 실패");
      }
      if (!payload.run_id) {
        throw new Error("토론 실행 식별자(run_id)를 받지 못했습니다.");
      }
      return payload;
    }

    async function requestDebateStop(runID) {
      const res = await fetch("/api/debate/stream/stop", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ run_id: runID })
      });
      const payload = await res.json();
      if (!res.ok) {
        throw new Error(payload.error || "토론 중지 실패");
      }
      return payload;
    }

    function buildPersonaGroups(defaultPath) {
      const merged = [{ label: "기본 그룹", path: defaultPath }].concat(predefinedGroups);
      const seenPath = new Set();
      personaGroups = merged.filter((group) => {
        const key = String(group.path || "");
        if (seenPath.has(key)) {
          return false;
        }
        seenPath.add(key);
        return true;
      });

      personaGroupEl.innerHTML = "";
      personaGroups.forEach((group) => {
        const option = document.createElement("option");
        option.value = group.path;
        option.textContent = group.label;
        personaGroupEl.appendChild(option);
      });
    }

    function getSelectedGroupLabel(path) {
      const selected = personaGroups.find((group) => group.path === path);
      if (!selected) {
        return "선택 그룹";
      }
      return selected.label;
    }

    function renderPersonaList(personas) {
      personaListEl.innerHTML = "";
      if (!personas || personas.length === 0) {
        const empty = document.createElement("div");
        empty.className = "placeholder small";
        empty.textContent = "표시할 persona가 없습니다.";
        personaListEl.appendChild(empty);
        return;
      }

      personas.forEach((persona, index) => {
        const item = document.createElement("article");
        item.className = "persona-item";
        item.dataset.personaId = persona.id || "";
        item.dataset.personaName = persona.name || "";

        const title = document.createElement("h3");
        title.className = "persona-title";
        title.textContent = String(index + 1) + ". " + (persona.name || persona.id || "Unnamed Persona");
        const head = document.createElement("div");
        head.className = "persona-head";

        const hue = hueFromText(persona.id || persona.name || String(index), index * 11);
        const avatar = document.createElement("span");
        avatar.className = "persona-avatar";
        avatar.textContent = initialsFromText(persona.name || persona.id || String(index + 1));
        avatar.style.backgroundColor = `hsl(${hue}, 60%, 90%)`;
        avatar.style.color = `hsl(${hue}, 70%, 30%)`;

        const role = document.createElement("p");
        role.className = "persona-role";
        role.textContent = persona.role || persona.stance || "(역할 설명 없음)";

        const master = document.createElement("p");
        master.className = "persona-master";
        master.textContent = "Master: " + (persona.master_name || "-");

        head.appendChild(avatar);
        head.appendChild(title);
        item.appendChild(head);
        item.appendChild(master);
        item.appendChild(role);
        personaListEl.appendChild(item);
      });
    }

    function createTurnCard(type, badge, name, content) {
      const card = document.createElement("article");
      card.className = "turn-card " + type;

      // Determine colors based on type
      let hue = 0;
      let sat = 0;
      let light = 95;
      
      if (type === "turn-persona") {
        hue = hueFromText(name);
        sat = 60;
        light = 90;
      } else if (type === "turn-moderator") {
        hue = 36; // Amber
        sat = 95;
        light = 94;
      } else if (type === "turn-system") {
        hue = 160; // Green-ish
        sat = 0; // Gray
        light = 92;
      } else if (type === "turn-summary") {
        hue = 210; // Blue
        sat = 80;
        light = 95;
      }

      const head = document.createElement("div");
      head.className = "turn-head";

      const nameWrap = document.createElement("div");
      nameWrap.className = "turn-name-wrap";

      const avatarEl = document.createElement("span");
      avatarEl.className = "turn-avatar";
      if (type === "turn-moderator") {
        avatarEl.textContent = "M";
        avatarEl.style.backgroundColor = "#ffedd5";
        avatarEl.style.color = "#9a3412";
      } else if (type === "turn-system") {
        avatarEl.textContent = "S";
        avatarEl.style.backgroundColor = "#e5e5ea";
        avatarEl.style.color = "#8e8e93";
      } else if (type === "turn-summary") {
        avatarEl.textContent = "R";
        avatarEl.style.backgroundColor = "#e0f2fe";
        avatarEl.style.color = "#0369a1";
      } else {
        avatarEl.textContent = initialsFromText(name);
        avatarEl.style.backgroundColor = `hsl(${hue}, ${sat}%, ${light}%)`;
        avatarEl.style.color = `hsl(${hue}, 70%, 30%)`;
      }

      const badgeEl = document.createElement("span");
      badgeEl.className = "turn-badge";
      badgeEl.textContent = badge;

      const nameEl = document.createElement("strong");
      nameEl.className = "turn-name";
      nameEl.textContent = name;

      nameWrap.appendChild(avatarEl);
      nameWrap.appendChild(nameEl);

      head.appendChild(nameWrap);
      head.appendChild(badgeEl);

      const contentEl = document.createElement("div");
      contentEl.className = "turn-content";
      contentEl.innerHTML = markdownToHTML(content);

      card.appendChild(head);
      card.appendChild(contentEl);
      return card;
    }

    function appendTurnCard(type, badge, name, content) {
      const card = createTurnCard(type, badge, name, content);
      card.style.setProperty("--turn-order", String(debateWindowEl.childElementCount + 1));
      debateWindowEl.appendChild(card);
      while (debateWindowEl.childElementCount > maxRenderedTurnCards) {
        debateWindowEl.removeChild(debateWindowEl.firstElementChild);
      }
      debateWindowEl.scrollTop = debateWindowEl.scrollHeight;
    }

	    function clearDebateWindow() {
	      debateWindowEl.innerHTML = "";
	      turnCount = 0;
	      setTurnMeta(0, "대기");
	      clearActivePersona();
	    }

	    function finalizeRunState(statusValue, turnState, errorMessage, stopNotice) {
	      if (typeof errorMessage === "string") {
	        errorText.textContent = errorMessage;
	      }
	      clearActivePersona();
	      statusText.textContent = statusValue;
	      setTurnMeta(turnCount, turnState);
	      setDebateRunning(false);
	      hideProgress();
	      closeCurrentStream();
	      currentRunID = "";
	      stopRequested = false;
	      if (stopNotice) {
	        appendTurnCard("turn-system", "STOP", "토론 중지", "사용자 요청으로 토론이 중지되었습니다.");
	      }
	    }

    function applyPersonaPayload(payload, path) {
      selectedPersonaPath = payload.path || path || "";
      personaMetaEl.textContent = getSelectedGroupLabel(path) + " · " + String(payload.personas.length) + "명";
      renderPersonaList(payload.personas);
    }

	    async function loadPersonasBySelectedGroup() {
	      const path = personaGroupEl.value.trim();
	      const seq = ++latestPersonaLoadSeq;
	      const payload = await fetchPersonas(path);
	      if (seq !== latestPersonaLoadSeq) {
	        return;
	      }
	      applyPersonaPayload(payload, path);
	    }

	    async function initPersonas() {
	      const seq = ++latestPersonaLoadSeq;
	      const payload = await fetchPersonas("");
	      if (seq !== latestPersonaLoadSeq) {
	        return;
	      }
	      const defaultPath = payload.path || "";
	      buildPersonaGroups(defaultPath);
	      personaGroupEl.value = defaultPath;
	      applyPersonaPayload(payload, defaultPath);
	    }


	    async function runDebate() {
	      errorText.textContent = "";
	      statusText.textContent = "토론 실행 중...";
	      setDebateRunning(true);
      stopRequested = false;
      showProgress("토론을 시작하는 중...");
      closeCurrentStream();
      currentRunID = "";

      try {
        if (typeof EventSource === "undefined") {
          throw new Error("이 브라우저는 SSE(EventSource)를 지원하지 않습니다.");
        }

        const problem = problemEl.value.trim();
        if (!problem) throw new Error("토론 주제를 입력해 주세요.");

	        const startPayload = await createDebateRun(problem);
	        currentRunID = String(startPayload.run_id);

	        const stream = new EventSource("/api/debate/stream?run_id=" + encodeURIComponent(currentRunID));
	        currentStream = stream;
	        const streamRunID = currentRunID;
	        let finished = false;
	        function isStaleStream() {
	          return currentStream !== stream || currentRunID !== streamRunID;
	        }

	        stream.addEventListener("start", function (ev) {
	          if (finished || isStaleStream()) {
	            return;
	          }
          const payload = parseJSON(ev.data) || {};
          clearDebateWindow();
          setTurnMeta(0, "진행 중");
          showProgress("토론 진행 중...");
          appendTurnCard(
            "turn-system",
            "START",
            "토론 시작",
            "주제: " + (payload.problem || problem) + "\n참여 persona 수: " + String(payload.persona_count || 0)
          );
        });

	        stream.addEventListener("turn", function (ev) {
	          if (finished || isStaleStream()) {
	            return;
	          }
          const turn = parseJSON(ev.data);
          if (!turn) {
            return;
          }
          const turnType = String(turn.type || "").toLowerCase();
          const isModerator = turnType === "moderator";
          if (isModerator) {
            clearActivePersona();
          } else {
            highlightSpeakerPersona(turn.speaker_id, turn.speaker_name);
          }
          turnCount += 1;
          setTurnMeta(turnCount, "진행 중");
          showProgress("토론 진행 중... (" + String(turnCount) + "턴)");
          appendTurnCard(
            isModerator ? "turn-moderator" : "turn-persona",
            (isModerator ? "MOD " : "TURN ") + String(turn.index || "?"),
            turn.speaker_name || turn.speaker_id || "Unknown",
            sanitizeTurnContent(turn.content || "")
          );
        });

        stream.addEventListener("complete", function (ev) {
          if (finished || isStaleStream()) {
            return;
          }
          finished = true;
          const payload = parseJSON(ev.data) || {};
          const result = payload.result || {};
          const consensus = result.consensus || {};
          const openRisks = Array.isArray(consensus.open_risks) ? consensus.open_risks.filter(Boolean) : [];
          const riskLine = openRisks.length > 0 ? openRisks.join(", ") : "-";
          const nextActionOwner = consensus.next_action_owner || "-";
          const nextActionTrigger = consensus.next_action_trigger_or_deadline || "-";
          const nextActionSuccessMetric = consensus.next_action_success_metric || "-";
          appendTurnCard(
            "turn-summary",
            "SUMMARY",
            "토론 결과",
            "status: " + (result.status || "-") + "\nconsensus_score: " + Number(consensus.score || 0).toFixed(2) +
              "\nsummary: " + (consensus.summary || "-") +
              "\nopen_risks: " + riskLine +
              "\nnext_action_owner: " + nextActionOwner +
              "\nnext_action_trigger_or_deadline: " + nextActionTrigger +
              "\nnext_action_success_metric: " + nextActionSuccessMetric +
              "\nrequired_next_action: " + (consensus.required_next_action || "-") +
              "\nsaved_json: " + (payload.saved_json_path || "-") +
              "\nsaved_markdown: " + (payload.saved_markdown_path || "-")
          );
          finalizeRunState("완료", "완료", "", false);
        });

	        stream.addEventListener("debate_error", function (ev) {
	          if (finished || isStaleStream()) {
	            return;
	          }
	          finished = true;
	          const payload = parseJSON(ev.data) || {};
	          finalizeRunState("실패", "실패", payload.error || "토론 실행 실패", false);
	        });

	        stream.addEventListener("stopped", function () {
	          if (finished || isStaleStream()) {
	            return;
	          }
	          finished = true;
	          finalizeRunState("중지됨", "중지", "", true);
	        });

	        stream.onerror = function () {
	          if (finished || isStaleStream()) {
	            return;
	          }
	          if (stopRequested) {
	            finished = true;
	            finalizeRunState("중지됨", "중지", "", true);
	            return;
	          }
	          if (stream.readyState === EventSource.CONNECTING) {
	            statusText.textContent = "재연결 중...";
	            showProgress("스트림 재연결 중...");
	            return;
	          }
	          finished = true;
	          const errMsg = errorText.textContent || "스트림 연결이 종료되었습니다.";
	          finalizeRunState("실패", "실패", errMsg, false);
	        };
	      } catch (err) {
	        finalizeRunState("실패", "실패", String(err.message || err), false);
	      }
	    }

    async function stopDebate() {
      if (!currentRunID) {
        return;
      }
      stopRequested = true;
      errorText.textContent = "";
      statusText.textContent = "중지 요청 중...";
      try {
        await requestDebateStop(currentRunID);
      } catch (err) {
        stopRequested = false;
        statusText.textContent = "토론 실행 중...";
        errorText.textContent = String(err.message || err);
        setDebateRunning(true);
      }
    }

    runBtn.addEventListener("click", runDebate);
    stopBtn.addEventListener("click", stopDebate);
    if (advancedResetBtn) {
      advancedResetBtn.addEventListener("click", () => {
        resetAdvancedOptions();
      });
    }
    personaGroupEl.addEventListener("change", async () => {
      try {
        errorText.textContent = "";
        await loadPersonasBySelectedGroup();
      } catch (err) {
        personaMetaEl.textContent = "";
        errorText.textContent = String(err.message || err);
      }
    });

    initPersonas().catch((err) => {
      personaMetaEl.textContent = "";
      errorText.textContent = String(err.message || err);
    });
    setTurnMeta(0, "대기");
    setDebateRunning(false);
    hideProgress();
