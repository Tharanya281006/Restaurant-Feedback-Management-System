(() => {
    "use strict";

    const formView = document.querySelector("#feedback-view");
    const allView = document.querySelector("#all-feedback-view");
    const navButtons = [...document.querySelectorAll(".nav-button")];
    const form = document.querySelector("#feedback-form");
    const message = document.querySelector("#message");
    const identityFields = document.querySelector("#identity-fields");
    const ratingInput = document.querySelector("#rating");
    const ratingButtons = [...document.querySelectorAll("#rating-buttons button")];
    const ratingLabel = document.querySelector("#rating-label");
    const comment = document.querySelector("#comment");
    const characterCount = document.querySelector("#character-count");
    const tableBody = document.querySelector("#feedback-body");
    const tableEmpty = document.querySelector("#table-empty");
    const searchInput = document.querySelector("#search-feedback");
    const pagination = document.querySelector("#pagination");
    const feedbackCount = document.querySelector("#feedback-count");
    const refreshButton = document.querySelector("#refresh-feedback");

    let allFeedback = [];
    let currentPage = 1;
    const pageSize = 8;

    function showMessage(text, success = false) {
        message.textContent = text;
        message.className = `message ${success ? "success" : "error"}`;
        message.hidden = false;
    }

    function clearMessage() {
        message.hidden = true;
        message.textContent = "";
    }

    function setView(view) {
        const showAll = view === "all-feedback";
        formView.hidden = showAll;
        allView.hidden = !showAll;
        navButtons.forEach(button => button.classList.toggle("active", button.dataset.view === view));
        if (showAll) loadAllFeedback();
    }

    navButtons.forEach(button => {
        button.addEventListener("click", () => setView(button.dataset.view));
    });



    // Use native radio inputs for reliable mouse, touch, and keyboard star selection.
    document.querySelectorAll('input[name="overall-star"]').forEach(input => {
        input.addEventListener("change", () => {
            const value = Number(input.value);
            ratingInput.value = String(value);
            document.querySelectorAll("#rating-buttons .star-choice").forEach(label => {
                const starValue = Number(label.htmlFor.replace("overall-", ""));
                label.classList.toggle("selected", starValue <= value);
            });
            ratingLabel.textContent = `${value} / 5 stars`;
        });
    });

    comment.addEventListener("input", () => {
        characterCount.textContent = String([...comment.value].length);
    });

    form.addEventListener("submit", async event => {
        event.preventDefault();
        clearMessage();

        if (!document.querySelector("#name").value.trim()) {
            showMessage("Please enter the customer name.");
            return;
        }
        const categoryIds = ["food-rating", "service-rating", "staff-rating", "cleanliness-rating", "ambience-rating", "value-rating"];
        if (categoryIds.some(id => !document.querySelector(`#${id}`).value)) {
            showMessage("Please rate all six restaurant experience areas.");
            return;
        }
        if (!ratingInput.value) {
            showMessage("Please select an overall rating.");
            return;
        }
        if (!comment.value.trim()) {
            showMessage("Please enter your feedback.");
            return;
        }

        const submit = form.querySelector("button[type=submit]");
        submit.disabled = true;
        try {
            const response = await fetch("/api/feedback", {
                method: "POST",
                body: new FormData(form)
            });
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || "Could not save feedback.");

            showMessage(`Feedback saved successfully. Reference: ${data.reference}`, true);
            form.reset();
            ratingInput.value = "";
            document.querySelectorAll('input[name="overall-star"]').forEach(input => { input.checked = false; });
            document.querySelectorAll("#rating-buttons .star-choice").forEach(star => star.classList.remove("selected"));
            ratingLabel.textContent = "Select a rating";
            characterCount.textContent = "0";
            identityFields.classList.remove("disabled");
        } catch (error) {
            showMessage(error.message || "Could not save feedback.");
        } finally {
            submit.disabled = false;
        }
    });

    function escapeHTML(value) {
        return String(value ?? "").replace(/[&<>'"]/g, char => ({
            "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;"
        }[char]));
    }

    function filteredFeedback() {
        const term = searchInput.value.trim().toLowerCase();
        if (!term) return allFeedback;
        return allFeedback.filter(item => [
            item.name, item.email, item.type, item.comment, item.reference, item.visitDate
        ].some(value => String(value || "").toLowerCase().includes(term)));
    }

    function renderTable() {
        const filtered = filteredFeedback();
        feedbackCount.textContent = `${filtered.length} ${filtered.length === 1 ? "feedback" : "feedbacks"}`;
        const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
        if (currentPage > totalPages) currentPage = totalPages;
        const start = (currentPage - 1) * pageSize;
        const rows = filtered.slice(start, start + pageSize);

        tableBody.innerHTML = rows.map((item, index) => {
            const displayName = item.name || "—";
            const email = item.email || "—";
            const rating = Number(item.rating) || 0;
            const stars = "★".repeat(rating) + "☆".repeat(Math.max(0, 5 - rating));
            const date = item.submittedAt ? new Date(item.submittedAt).toLocaleString() : "—";
            const visitDate = item.visitDate ? escapeHTML(item.visitDate) : "—";
            const ratings = item.ratings || {};
            const cellRating = key => ratings[key] ? `${escapeHTML(ratings[key])}/5` : "—";
            return `<tr>
                <td>${start + index + 1}</td>
                <td><strong>${escapeHTML(displayName)}</strong></td>
                <td>${escapeHTML(email)}</td>
                <td>${cellRating("food")}</td>
                <td>${cellRating("service")}</td>
                <td>${cellRating("staff")}</td>
                <td>${cellRating("cleanliness")}</td>
                <td>${cellRating("ambience")}</td>
                <td>${cellRating("value")}</td>
                <td class="stars-cell" title="${rating}/5">${stars}</td>
                <td class="comment-cell" title="${escapeHTML(item.comment || "")}">${escapeHTML(item.comment || "—")}</td>
                <td>${visitDate}</td>
                <td>${escapeHTML(date)}</td>
            </tr>`;
        }).join("");

        tableEmpty.hidden = rows.length !== 0;
        renderPagination(totalPages);
    }

    function renderPagination(totalPages) {
        pagination.innerHTML = "";
        if (totalPages <= 1) return;
        const previous = document.createElement("button");
        previous.textContent = "Previous";
        previous.disabled = currentPage === 1;
        previous.addEventListener("click", () => { currentPage--; renderTable(); });
        pagination.append(previous);

        for (let page = 1; page <= totalPages; page++) {
            const button = document.createElement("button");
            button.textContent = String(page);
            button.className = page === currentPage ? "active" : "";
            button.addEventListener("click", () => { currentPage = page; renderTable(); });
            pagination.append(button);
        }

        const next = document.createElement("button");
        next.textContent = "Next";
        next.disabled = currentPage === totalPages;
        next.addEventListener("click", () => { currentPage++; renderTable(); });
        pagination.append(next);
    }

    async function loadAllFeedback() {
        tableBody.innerHTML = "";
        tableEmpty.hidden = false;
        tableEmpty.textContent = "Loading feedback...";
        try {
            const response = await fetch("/api/feedback/all", { cache: "no-store" });
            if (!response.ok) throw new Error("Could not load feedback.");
            allFeedback = await response.json();
            currentPage = 1;
            renderTable();
        } catch (error) {
            tableEmpty.hidden = false;
            tableEmpty.textContent = error.message;
            feedbackCount.textContent = "0 feedback";
            pagination.innerHTML = "";
        }
    }

    searchInput.addEventListener("input", () => {
        currentPage = 1;
        renderTable();
    });

    refreshButton.addEventListener("click", loadAllFeedback);
})();
