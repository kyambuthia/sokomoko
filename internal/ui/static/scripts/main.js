document.addEventListener("DOMContentLoaded", () => {
    console.log("DOMContentLoaded")

    const searchForm = document.querySelector("#searchForm");

    if (searchForm) {
        searchForm.addEventListener("submit", async (e) => {
            e.preventDefault();

            const formData = new FormData(searchForm);
            const queryString = formData.get("queryString")?.trim();
            
            if (!queryString) {
                return;
            }

            // Redirect to search page with query parameter for simple navigation
            window.location.href = `/search?q=${encodeURIComponent(queryString)}`;
        });   
    }

    // Handle search input with debouncing for live search (optional enhancement)
    const searchInput = document.querySelector("#searchInput");
    if (searchInput) {
        let debounceTimer;
        
        searchInput.addEventListener("input", async (e) => {
            clearTimeout(debounceTimer);
            const query = e.target.value.trim();
            
            if (query.length < 2) {
                return; // Don't search for very short queries
            }
            
            debounceTimer = setTimeout(async () => {
                try {
                    const response = await fetch("/search", {
                        method: "POST",
                        headers: {
                            "Content-Type": "application/json"
                        },
                        body: JSON.stringify({ queryString: query })
                    });
                    
                    if (!response.ok) {
                        throw new Error(`HTTP error! status: ${response.status}`);
                    }
                    
                    const result = await response.json();
                    console.log("Search results:", result);
                    
                    // You could update a dropdown with results here if needed
                    // For now, we'll just log them
                    
                } catch (error) {
                    console.error("Search failed:", error);
                }
            }, 300); // 300ms debounce
        });
    }
});