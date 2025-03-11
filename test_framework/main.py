import requests
import time
from flask import Flask, jsonify, request

app = Flask(__name__)

SWITCH_SERVER = "http://localhost:8080"

# Simulate sending updates


@app.route("/set_dashboard_active", methods=["GET"])
def set_dashboard_active():
    return set_page_active("dashboard", 5, session_id="user_325")

@app.route("/set_settings_active", methods=["GET"])
def set_settings_active():
    return set_page_active("settings", 10, session_id="user_893")

@app.route("/set_profile_active", methods=["GET"])
def set_profile_active():
    return set_page_active("profile", 3, session_id="user_246")

@app.route("/set_notifications_active", methods=["GET"])
def set_notifications_active():
    return set_page_active("notifications", 8, session_id="user_123")

# Helper function to set page as active with timeout
def set_page_active(page, timeout, session_id):
    print(f"Marking {page} as active for session {session_id} with a timeout of {timeout} seconds")

    # Notify Switch that the page is active with the specific timeout
    try:
        response = requests.post(f"{SWITCH_SERVER}/set_active", params={"session_id": session_id, "page": page, "timeout": timeout})
        if response.status_code == 200:
            return jsonify({"page": page, "timeout": timeout, "status": "success"}), 200
        else:
            return jsonify({"page": page, "timeout": timeout, "status": "failed", "error": response.text}), 500
    except Exception as e:
        return jsonify({"page": page, "timeout": timeout, "status": "failed", "error": str(e)}), 500

if __name__ == "__main__":
    app.run(debug=True)