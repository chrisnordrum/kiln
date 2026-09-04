// The Kiln client. It is the same for every program: it carries no
// application logic, so there is nothing per-app to debug here.
//
// A control declares what it does in data attributes. This script posts that
// to the server, which re-runs the route's data block and returns fresh markup
// to swap in. All state lives on the server.
(function () {
  function argsOf(el) {
    try { return JSON.parse(el.dataset.kArgs || "{}"); } catch (e) { return {}; }
  }

  async function send(name, args) {
    let out;
    try {
      const res = await fetch("/_k/action", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ action: name, args: args, path: location.pathname }),
      });
      out = await res.json();
    } catch (e) {
      return note("could not reach the server", true);
    }
    if (out.error) return note(out.error, true);
    if (out.redirect) { location.href = out.redirect; return; }
    if (out.html) {
      const main = document.querySelector("main");
      if (main) main.outerHTML = out.html;
    }
    if (out.toast) note(out.toast, false);
  }

  function note(text, isError) {
    const el = document.createElement("div");
    el.className = "k-toast" + (isError ? " k-toast-error" : "");
    el.textContent = text;
    document.body.appendChild(el);
    setTimeout(function () { el.remove(); }, 4000);
  }

  document.addEventListener("click", function (e) {
    const el = e.target.closest("[data-k-do]");
    if (!el || el.tagName === "FORM" || el.type === "checkbox") return;
    e.preventDefault();
    if (el.dataset.kConfirm && !confirm(el.dataset.kConfirm)) return;
    send(el.dataset.kDo, argsOf(el));
  });

  document.addEventListener("change", function (e) {
    const el = e.target.closest("[data-k-do]");
    if (!el || el.type !== "checkbox") return;
    const args = argsOf(el);
    if (el.dataset.kEvent) args[el.dataset.kEvent] = el.checked;
    send(el.dataset.kDo, args);
  });

  document.addEventListener("submit", function (e) {
    const form = e.target.closest("form[data-k-do]");
    if (!form) return;
    e.preventDefault();
    const args = argsOf(form);
    new FormData(form).forEach(function (v, k) { args[k] = v; });
    send(form.dataset.kDo, args);
    form.reset();
  });
})();
