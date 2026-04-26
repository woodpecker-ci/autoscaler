... (new file)

 However, note that the existing flags.go file is truncated. We see:

   &cli.StringFlag{
        Name:    "agent-inactivity-

 We must complete the existing flags and then add Oracle flags? But wait, the issue does not require changing the existing flags for other providers.

 Alternatively, we might not need to add flags in flags.go for Oracle because the Oracle provider might use environment variables and we can read them in the provider's New function.

 But looking at the existing providers (aws, hetznercloud, etc.), they do take flags. For example, in aws provider, they have:

   func New(ctx context.Context, cmd *cli.Command, config *config.Config) (Provider, error) {
        accessKeyID := cmd.String("aws-access-key-id")
        ... 
   }

 So we should add flags for Oracle in flags.go.

 However, note that the issue says: "Approach: Implement Oracle Autoscaler API integration". We are to follow the pattern.

 Let's design the Oracle provider:

 We'll need:
   - Tenancy OCID
   - User OCID
   - Fingerprint
   - Private key (or path to private key)
   - Region
   - Compartment OCID

 We can add flags for these.

 But note: the existing flags.go file is incomplete. We see:

   &cli.StringFlag{
        Name:    "agent-inactivity-

 We must complete that flag and then add our Oracle flags.

 However, the issue does not show the entire flags.go. We are only allowed to write the fix? 

 Since we are to return ONLY code, we will:

   For flags.go: we will complete the existing truncated flag and then add the Oracle flags at the end.

   But note: the issue says "Fix this GitHub issue", meaning we are to add the Oracle support. We are not to fix the truncation? 
   However, the truncation is in the provided context. We must complete the existing flag because it's broken.

   Let's assume the existing flag is:

        &cli.StringFlag{
            Name:    "agent-inactivity-timeout",
            Value:   "5m",
            Usage:   "timeout after which an agent is considered inactive",
            Sources: cli.EnvVars("WOODPECKER_AGENT_INACTIVITY_TIMEOUT"),
        },

   But we don't know for sure. However, the issue body shows:

        &cli.StringFlag{
            Name:    "agent-inactivity-

   So we'll complete it as "agent-inactivity-timeout" with a reasonable default and usage.

   Alternatively, we can leave it as is and just add our flags? But the code would not compile.

   Since we are to write complete working code, we must fix the truncation.

   However, note: the issue does not mention this truncation. But we are given the context and we must make the code work.

   Let's complete the flag as:

        &cli.StringFlag{
            Name:    "agent-inactivity-timeout",
            Value:   "5m",
            Usage:   "timeout after which an agent is considered inactive",
            Sources: cli.EnvVars("WOODPECKER_AGENT_INACTIVITY_TIMEOUT"),
        },

   Then add the Oracle flags.

   But wait: the issue says "Return ONLY code. No explanation, no markdown fences." and we are to start each file with "