const errorEl = document.getElementById("error");
const serviceForm = document.getElementById("service-form");
const incidentForm = document.getElementById("incident-form");
const serviceReset = document.getElementById("service-reset");
const incidentReset = document.getElementById("incident-reset");
const serviceId = serviceForm.querySelector('[name="id"]');
const incidentId = incidentForm.querySelector('[name="id"]');

let services = [];

function showError(msg) {
  errorEl.hidden = !msg;
  errorEl.textContent = msg || "";
}

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  if (res.status === 204) return null;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

function fillServiceSelect() {
  const select = incidentForm.service_id;
  const current = select.value;
  select.innerHTML = services
    .map((s) => `<option value="${s.id}">${s.id}: ${escapeHtml(s.name)}</option>`)
    .join("");
  if (current) select.value = current;
}

function escapeHtml(s) {
  return String(s)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

async function loadAll() {
  showError("");
  services = await api("/api/services");
  fillServiceSelect();
  document.getElementById("services-body").innerHTML = services
    .map(
      (s) => `<tr>
        <td>${s.id}</td>
        <td>${escapeHtml(s.name)}</td>
        <td>${escapeHtml(s.owner)}</td>
        <td>${escapeHtml(s.description || "")}</td>
        <td class="row-actions">
          <button data-edit-service="${s.id}">Изменить</button>
          <button data-del-service="${s.id}">Удалить</button>
        </td>
      </tr>`
    )
    .join("");

  const incidents = await api("/api/incidents");
  const nameById = Object.fromEntries(services.map((s) => [s.id, s.name]));
  document.getElementById("incidents-body").innerHTML = incidents
    .map(
      (i) => `<tr>
        <td>${i.id}</td>
        <td>${escapeHtml(nameById[i.service_id] || i.service_id)}</td>
        <td>${escapeHtml(i.title)}</td>
        <td>${escapeHtml(i.severity)}</td>
        <td>${escapeHtml(i.status)}</td>
        <td class="row-actions">
          <button data-edit-incident="${i.id}">Изменить</button>
          <button data-del-incident="${i.id}">Удалить</button>
        </td>
      </tr>`
    )
    .join("");
}

serviceForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const fd = new FormData(serviceForm);
  const payload = {
    name: fd.get("name"),
    owner: fd.get("owner"),
    description: fd.get("description") || "",
  };
  try {
    const id = serviceId.value;
    if (id) {
      await api(`/api/services/${id}`, { method: "PUT", body: JSON.stringify(payload) });
    } else {
      await api("/api/services", { method: "POST", body: JSON.stringify(payload) });
    }
    serviceForm.reset();
    serviceId.value = "";
    serviceReset.hidden = true;
    await loadAll();
  } catch (err) {
    showError(err.message);
  }
});

incidentForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const fd = new FormData(incidentForm);
  const payload = {
    service_id: Number(fd.get("service_id")),
    title: fd.get("title"),
    description: fd.get("description") || "",
    severity: fd.get("severity"),
    status: fd.get("status"),
  };
  try {
    const id = incidentId.value;
    if (id) {
      await api(`/api/incidents/${id}`, { method: "PUT", body: JSON.stringify(payload) });
    } else {
      await api("/api/incidents", { method: "POST", body: JSON.stringify(payload) });
    }
    incidentForm.reset();
    incidentId.value = "";
    incidentReset.hidden = true;
    await loadAll();
  } catch (err) {
    showError(err.message);
  }
});

serviceReset.addEventListener("click", () => {
  serviceForm.reset();
  serviceId.value = "";
  serviceReset.hidden = true;
});

incidentReset.addEventListener("click", () => {
  incidentForm.reset();
  incidentId.value = "";
  incidentReset.hidden = true;
  fillServiceSelect();
});

document.body.addEventListener("click", async (e) => {
  const t = e.target;
  try {
    if (t.dataset.editService) {
      const s = await api(`/api/services/${t.dataset.editService}`);
      serviceId.value = s.id;
      serviceForm.name.value = s.name;
      serviceForm.owner.value = s.owner;
      serviceForm.description.value = s.description;
      serviceReset.hidden = false;
    }
    if (t.dataset.delService) {
      if (!confirm("Удалить сервис и связанные инциденты?")) return;
      await api(`/api/services/${t.dataset.delService}`, { method: "DELETE" });
      await loadAll();
    }
    if (t.dataset.editIncident) {
      const i = await api(`/api/incidents/${t.dataset.editIncident}`);
      incidentId.value = i.id;
      incidentForm.service_id.value = i.service_id;
      incidentForm.title.value = i.title;
      incidentForm.description.value = i.description;
      incidentForm.severity.value = i.severity;
      incidentForm.status.value = i.status;
      incidentReset.hidden = false;
    }
    if (t.dataset.delIncident) {
      if (!confirm("Удалить инцидент?")) return;
      await api(`/api/incidents/${t.dataset.delIncident}`, { method: "DELETE" });
      await loadAll();
    }
  } catch (err) {
    showError(err.message);
  }
});

loadAll().catch((err) => showError(err.message));
