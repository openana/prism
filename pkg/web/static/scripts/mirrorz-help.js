// LLM usage: this script is generated with deepseek-v4-pro.
/**
 * mirrorz-help.js — Client-side mustache.js rendering for MirrorZ help pages.
 *
 * The generated template embeds a hidden <input data-var="endpoint" data-global>
 * with value="{{ .Endpoint }}" (filled by Go server-side).
 * This script reads it, builds a mustache context, and renders all code blocks.
 */
(function () {
  'use strict';

  // Disable Mustache HTML escaping — we render into textContent, not innerHTML.
  if (typeof Mustache !== 'undefined') {
    Mustache.escape = function (text) { return text; };
  }

  var endpointURL;

  function initEndpoint() {
    var el = document.querySelector('[data-var="endpoint"][data-global]');
    var raw = el ? el.value : '';
    try { endpointURL = new URL(raw); }
    catch (e) { endpointURL = { protocol: 'https:', host: '', pathname: '' }; }
  }

  // Walk up the DOM to find which template group an element belongs to.
  // Returns the data-template value of the nearest ancestor .code-block-inputs, or null.
  function findTemplateGroup(el) {
    var parent = el.parentElement;
    while (parent) {
      if (parent.hasAttribute('data-template') &&
        parent.classList.contains('code-block-inputs')) {
        return parent.getAttribute('data-template');
      }
      parent = parent.parentElement;
    }
    return null;
  }

  // Merge all [data-var] inputs inside a container element into ctx.
  function mergeInputs(ctx, container) {
    var items = container.querySelectorAll('[data-var]');
    for (var i = 0; i < items.length; i++) {
      var el = items[i];
      var vn = el.getAttribute('data-var');
      if (!vn || vn === 'endpoint') continue;
      if (el.type === 'checkbox') {
        ctx[vn] = el.checked ? parseVal(el.getAttribute('data-true')) : parseVal(el.getAttribute('data-false'));
      } else if (el.tagName === 'SELECT') {
        ctx[vn] = el.value;
        var opt = el.options[el.selectedIndex];
        if (opt) {
          var ex = opt.getAttribute('data-extras');
          if (ex) { try { var x = JSON.parse(ex); for (var k in x) { if (x.hasOwnProperty(k)) ctx[k] = parseVal(x[k]); } } catch (e) { } }
        }
      } else { ctx[vn] = el.value; }
    }
  }

  // Build the shared context: endpoint-derived fields + sudo + ungrouped inputs + global-menu inputs.
  function buildSharedContext() {
    var scheme = endpointURL.protocol.replace(':', '');
    var host = endpointURL.host;
    var path = endpointURL.pathname.replace(/\/$/, '');
    var endpoint = scheme + '://' + host + path;

    var ctx = {
      scheme: scheme, host: host, path: path, endpoint: endpoint,
      sudo: getSudoEnabled() ? 'sudo ' : '',
      sudoE: getSudoEnabled() ? 'sudo -E ' : ''
    };

    // Ungrouped inputs (not inside any .code-block-inputs[data-template]) are shared.
    var allInputs = document.querySelectorAll('[data-var]:not([data-global])');
    for (var i = 0; i < allInputs.length; i++) {
      var el = allInputs[i];
      var vn = el.getAttribute('data-var');
      if (!vn || vn === 'endpoint') continue;
      if (findTemplateGroup(el)) continue; // belongs to a template group — skip
      if (el.type === 'checkbox') {
        ctx[vn] = el.checked ? parseVal(el.getAttribute('data-true')) : parseVal(el.getAttribute('data-false'));
      } else if (el.tagName === 'SELECT') {
        ctx[vn] = el.value;
        var opt = el.options[el.selectedIndex];
        if (opt) {
          var ex = opt.getAttribute('data-extras');
          if (ex) { try { var x = JSON.parse(ex); for (var k in x) { if (x.hasOwnProperty(k)) ctx[k] = parseVal(x[k]); } } catch (e) { } }
        }
      } else { ctx[vn] = el.value; }
    }

    // Global-menu inputs are always shared.
    var menus = document.querySelectorAll('[data-global-menu]');
    for (var g = 0; g < menus.length; g++) {
      mergeInputs(ctx, menus[g]);
    }
    return ctx;
  }

  function parseVal(v) { if (v === 'true') return true; if (v === 'false') return false; return v; }
  function getSudoEnabled() { var e = document.querySelector('.sudo-toggle'); return e ? e.checked : true; }
  function getHttpsEnabled() { var e = document.querySelector('.https-toggle'); return e ? e.checked : true; }

  function renderAll() {
    var sharedCtx = buildSharedContext();
    var targets = document.querySelectorAll('pre code[data-template]');
    for (var i = 0; i < targets.length; i++) {
      var t = targets[i];
      var tid = t.getAttribute('data-template');
      if (!tid) continue;
      var tpl = document.getElementById(tid);
      if (!tpl) continue;

      // Start with a copy of the shared context.
      var ctx = {};
      for (var k in sharedCtx) { if (sharedCtx.hasOwnProperty(k)) ctx[k] = sharedCtx[k]; }

      // Merge in inputs specific to this template group.
      var group = document.querySelector('.code-block-inputs[data-template="' + tid + '"]');
      if (group) { mergeInputs(ctx, group); }

      try {
        t.textContent = Mustache.render(tpl.textContent || tpl.innerHTML, ctx).replace(/\n+$/, '');
      } catch (e) { t.textContent = '[Render error: ' + e.message + ']'; }
    }
  }

  function bindEvents() {
    var ctrls = document.querySelectorAll('[data-var]:not([data-global])');
    for (var i = 0; i < ctrls.length; i++) {
      var el = ctrls[i];
      el.addEventListener('change', function () {
        if (this.type === 'checkbox') syncCheckboxes(this);
        renderAll();
      });
      el.addEventListener('input', renderAll);
    }
    var st = document.querySelector('.sudo-toggle');
    if (st) st.addEventListener('change', renderAll);
    var ht = document.querySelector('.https-toggle');
    if (ht) ht.addEventListener('change', function () { endpointURL.protocol = (this.checked ? 'https' : 'http') + ':'; renderAll(); });
  }

  function syncCheckboxes(changed) {
    var name = changed.getAttribute('data-var');
    if (!name) return;
    var changedGroup = findTemplateGroup(changed);
    var candidates = document.querySelectorAll('[data-var="' + name + '"]');
    for (var i = 0; i < candidates.length; i++) {
      if (candidates[i] !== changed && candidates[i].type === 'checkbox') {
        // Only sync checkboxes that belong to the same template group.
        if (findTemplateGroup(candidates[i]) !== changedGroup) continue;
        candidates[i].checked = changed.checked;
      }
    }
  }

  initEndpoint();
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { initEndpoint(); renderAll(); bindEvents(); });
  } else { initEndpoint(); renderAll(); bindEvents(); }
})();
