//! Dex Thin Client - Tauri backend
//!
//! Provides native functionality for the thin client:
//! - Secure credential storage (keychain)
//! - System tray integration
//! - Native notifications

use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Manager, Runtime,
};

#[cfg(not(any(target_os = "android", target_os = "ios")))]
use keyring::Entry;

/// Save auth key to system keychain
#[tauri::command]
async fn save_auth_key(key: String) -> Result<(), String> {
    #[cfg(not(any(target_os = "android", target_os = "ios")))]
    {
        let entry = Entry::new("dex", "auth-key").map_err(|e| e.to_string())?;
        entry.set_password(&key).map_err(|e| e.to_string())?;
        Ok(())
    }

    #[cfg(any(target_os = "android", target_os = "ios"))]
    {
        // On mobile, we'll use Tauri's storage or a secure storage plugin
        // For now, just succeed (we can implement proper secure storage later)
        let _ = key;
        Ok(())
    }
}

/// Get auth key from system keychain
#[tauri::command]
async fn get_auth_key() -> Result<Option<String>, String> {
    #[cfg(not(any(target_os = "android", target_os = "ios")))]
    {
        let entry = Entry::new("dex", "auth-key").map_err(|e| e.to_string())?;
        match entry.get_password() {
            Ok(key) => Ok(Some(key)),
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(e) => Err(e.to_string()),
        }
    }

    #[cfg(any(target_os = "android", target_os = "ios"))]
    {
        // On mobile, return None for now
        Ok(None)
    }
}

/// Delete auth key from system keychain
#[tauri::command]
async fn delete_auth_key() -> Result<(), String> {
    #[cfg(not(any(target_os = "android", target_os = "ios")))]
    {
        let entry = Entry::new("dex", "auth-key").map_err(|e| e.to_string())?;
        match entry.delete_credential() {
            Ok(()) => Ok(()),
            Err(keyring::Error::NoEntry) => Ok(()), // Already deleted
            Err(e) => Err(e.to_string()),
        }
    }

    #[cfg(any(target_os = "android", target_os = "ios"))]
    {
        Ok(())
    }
}

/// Set up the system tray
fn setup_tray<R: Runtime>(app: &tauri::App<R>) -> Result<(), Box<dyn std::error::Error>> {
    let quit = MenuItem::with_id(app, "quit", "Quit Dex", true, None::<&str>)?;
    let show = MenuItem::with_id(app, "show", "Show Dex", true, None::<&str>)?;
    let menu = Menu::with_items(app, &[&show, &quit])?;

    let _tray = TrayIconBuilder::new()
        .icon(app.default_window_icon().unwrap().clone())
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app, event| match event.id.as_ref() {
            "quit" => {
                app.exit(0);
            }
            "show" => {
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.show();
                    let _ = window.set_focus();
                }
            }
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                let app = tray.app_handle();
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.show();
                    let _ = window.set_focus();
                }
            }
        })
        .build(app)?;

    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![
            save_auth_key,
            get_auth_key,
            delete_auth_key,
        ])
        .setup(|app| {
            // Set up tray on desktop only
            #[cfg(not(any(target_os = "android", target_os = "ios")))]
            {
                setup_tray(app)?;
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
