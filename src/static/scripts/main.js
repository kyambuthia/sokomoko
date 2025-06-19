window.onload = () =>
{
    const searchForm = document.querySelector("#searchForm")

    searchForm.addEventListener("submit", async (e) => {
        e.preventDefault();
        const form = e.target;
        const data = {
            searchQuery: form.value.queryString
        }

        const res = await fetch("/search", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify(data)
        });

        const text = await res.text();
        console.log(text)

    });
}
