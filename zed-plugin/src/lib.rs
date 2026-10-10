use zed_extension_api::{self as zed, settings::LspSettings};

struct ArdExtension;

const LANGUAGE_SERVER: &str = "ard-lsp";

impl zed::Extension for ArdExtension {
    fn new() -> Self {
        Self
    }

    /// Resolves the language server binary in this order:
    /// 1. `lsp.ard-lsp.binary.path` from Zed settings, with optional arguments;
    /// 2. an `ard-dev` development build on PATH;
    /// 3. the installed `ard` on PATH.
    fn language_server_command(
        &mut self,
        _language_server_id: &zed::LanguageServerId,
        worktree: &zed::Worktree,
    ) -> zed::Result<zed::Command> {
        let binary = LspSettings::for_worktree(LANGUAGE_SERVER, worktree)
            .ok()
            .and_then(|settings| settings.binary);
        let configured_path = binary.as_ref().and_then(|binary| binary.path.clone());
        let args = binary
            .and_then(|binary| binary.arguments)
            .unwrap_or_else(|| vec!["lsp".to_string()]);

        let ard_path = configured_path
            .or_else(|| worktree.which("ard-dev"))
            .or_else(|| worktree.which("ard"))
            .ok_or_else(|| {
                "ard binary not found in PATH. Install via: cd compiler && go build -o ard"
                    .to_string()
            })?;

        Ok(zed::Command {
            command: ard_path,
            args,
            env: Default::default(),
        })
    }
}

zed::register_extension!(ArdExtension);
