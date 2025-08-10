document.addEventListener("DOMContentLoaded", (e) => {
    console.log("DOMContentloaded")

    const searchForm = document.querySelector("#searchForm");

    if (searchForm)
    {
        searchForm.addEventListener("submit", async (e) => {
            e.preventDefault();

            const formData = new FormData(searchForm);
            // DEBUGS 
            //  ---
            //console.log(formData.get("queryString"));
            const data = {
                queryString: formData.get("queryString")
            }

            //console.log(JSON.stringify(data));
            const res = await fetch("/search", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify(data)
            });

            const text = await res.text();
            console.log("response text: \n", text)
        });   
    }

    // read values from the search-form.
    async function readSearchFormValues()
    {
        const url = "/search";
        try
        {
            const res =  await fetch(url)
            if (!res.ok)
            {
                throw Error(`RESPONSE: ${res.status}`)
            }
            const content = await res.json();
            console.log(json);
        }
        catch(err)
        {
            console.error(err.message);
        }
    }

    // --> load JSON from server.
    readSearchFormValues();
})