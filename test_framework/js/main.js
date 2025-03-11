const express = require("express");
const axios = require("axios");

const app = express();
const PORT = 3000;
const GATE_SERVER = "http://localhost:6748";

// Helper function to set page as active with timeout
const setPageActive = async (page, timeout, sessionId) => {
    console.log(`Marking ${page} as active for session ${sessionId} with a timeout of ${timeout} seconds`);
    
    try {
        const response = await axios.post(`${GATE_SERVER}/set_active`, null, {
            params: { session_id: sessionId, page, timeout }
        });
        
        if (response.status === 200) {
            return { page, timeout, status: "success" };
        } else {
            return { page, timeout, status: "failed", error: response.data };
        }
    } catch (error) {
        return { page, timeout, status: "failed", error: error.message };
    }
};

app.get("/set_dashboard_active", async (req, res) => {
    const result = await setPageActive("dashboard", 5, "user_325");
    res.status(result.status === "success" ? 200 : 500).json(result);
});

app.get("/set_settings_active", async (req, res) => {
    const result = await setPageActive("settings", 10, "user_893");
    res.status(result.status === "success" ? 200 : 500).json(result);
});

app.get("/set_profile_active", async (req, res) => {
    const result = await setPageActive("profile", 3, "user_246");
    res.status(result.status === "success" ? 200 : 500).json(result);
});

app.get("/set_notifications_active", async (req, res) => {
    const result = await setPageActive("notifications", 8, "user_123");
    res.status(result.status === "success" ? 200 : 500).json(result);
});

app.listen(PORT, () => {
    console.log(`Server running on port ${PORT}`);
});
