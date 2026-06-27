use zed_extension_api as zed;

struct ArdExtension;

impl zed::Extension for ArdExtension {
    fn new() -> Self {
        Self
    }

    fn language_server_command(
        &mut self,
        _language_server_id: &zed::LanguageServerId,
        worktree: &zed::Worktree,
    ) -> zed::Result<zed::Command> {
        let ard_path = worktree
            .which("ard-dev")
            .ok_or_else(|| "ard binary not found in PATH. Install via: cd compiler && go build -o ard".to_string())?;

        Ok(zed::Command {
            command: ard_path,
            args: vec!["lsp".to_string()],
            env: Default::default(),
        })
    }
}

zed::register_extension!(ArdExtension);
