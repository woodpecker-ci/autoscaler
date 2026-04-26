... (truncated)

   We are not given the entire files. We are to fix the issue by adding the necessary code in the appropriate places.

   Since we are to add Oracle support, we must:

   1. In flags.go: add the Oracle flags (and fix the truncation if necessary).
   2. In main.go: add the Oracle case in setupProvider.
   3. Create a new file: providers/oracle.go.

   But note: the issue does not show the entire flags.go, so we cannot see the rest of the flags. We are only given a part.

   We have two options:

   Option 1: Assume that the flags.go file ends after the truncated flag and we are to complete that flag and then add our flags.

   Option 2: Since we are only to fix the issue, we can just add the Oracle flags at the end of the existing flags (without touching the truncation) and hope that the truncation is a mistake in the issue body?

   However, the truncation is in the issue body. We must complete the flag to make the code compile.

   Let's look at the provided flags.go snippet:

        &cli.StringFlag{
            Name:    "agent-inactivity-

   We don't know the rest. But we can complete it by guessing from the context of the other flags and the config.

   In config.go, we see:

        AgentInactivityTimeout time.Duration

   So the flag should be for AgentInactivityTimeout.

   We'll complete it as:

        &cli.StringFlag{
            Name:    "agent-inactivity-timeout",
            Value:   "5m",
            Usage:   "timeout after which an agent is considered inactive",
            Sources: cli.EnvVars("WOODPECKER_AGENT_INACTIVITY_TIMEOUT"),
        },

   Then we add the Oracle flags.

   For Oracle, we'll add:

        &cli.StringFlag{
            Name:    " oracle-tenancy-ocid",
            Value:   "",
            Usage:   "OCI tenancy OCID",
            Sources: cli.EnvVars("WOODPECKER_ORACLE_TENANCY_OCID"),
        },
        &cli.StringFlag{
            Name:    " oracle-user-ocid",
            Value:   "",
            Usage:   "OCI user OCID",
            Sources: cli.EnvVars("WOODPECKER_ORACLE_USER_OCID"),
        },
        &cli.StringFlag{
            Name:    " oracle-fingerprint",
            Value:   "",
            Usage:   "OCI API key fingerprint",
            Sources: cli.EnvVars("WOODPECKER_ORACLE_FINGERPRINT"),
        },
        &cli.StringFlag{
            Name:    " oracle-private-key",
            Value:   "",
            Usage:   "OCI API private key (or path to private key file)",
            Sources: cli.EnvVars("WOODPECKER_ORACLE_PRIVATE_KEY"),
        },
        &cli.StringFlag{
            Name:    " oracle-region",
            Value:   "",
            Usage:   "OCI region",
            Sources: cli.EnvVars("WOODPECKER_ORACLE_REGION"),
        },
        &cli.StringFlag{
            Name:    " oracle-compartment-ocid",
            Value:   "",
            Usage:   "OCI compartment OCID",
            Sources: cli.EnvVars("WOODPECKER_ORACLE_COMPARTMENT_OCID"),
        },

   However, note: the existing flags use kebab-case and without spaces. We should use the same style.

   Also, note that the existing flags do not have a space after the name. We'll do:

        Name:    "oracle-tenancy-ocid",

   But wait: the existing flags are like:

        Name:    "log-level",

   So we use kebab-case.

   Now, for main.go: we add a case for "oracle" in the switch statement in setupProvider.

        case "oracle":
            return oracle.New(ctx, cmd, config)

   And we must import the oracle provider.

   Then, we create the file providers/oracle.go.

   In providers/oracle.go, we implement the Provider interface.

   We'll define:

        type OracleProvider struct {
            // ... fields for the configuration
        }

        func New(ctx context.Context, cmd *cli.Command, config *config.Config) (Provider, error) {
            // read the flags from cmd
            tenancyOCID := cmd.String("oracle-tenancy-ocid")
            // ... etc.

            // validate
            // create the OCI client
            // return &OracleProvider{ ... }
        }

        func (o *OracleProvider) CreateAgent(ctx context.Context, config *config.Config) error {
            // create an instance in OCI
        }

        func (o *OracleProvider) DeleteAgent(ctx context.Context, agent *woodpecker.Agent) error {
            // terminate the instance
        }

        func (o *OracleProvider) GetAgent(ctx context.Context, agentID string) (*woodpecker.Agent, error) {
            // get the instance by ID and convert to woodpecker.Agent
        }

        func (o *OracleProvider) ListAgents(ctx context.Context) ([]*woodpecker.Agent, error) {
            // list instances in the compartment that are our agents (by tag or display name?)
        }

   However, note: the existing providers (like aws) have a different structure. We must mimic that.

   Looking at the aws provider (not provided, but we can infer from the usage in main.go and the fact that it returns a Provider):

        type Provider interface {
            CreateAgent(context.Context, *config.Config) error
            DeleteAgent(context.Context, *woodpecker.Agent) error
            GetAgent(context.Context, string) (*woodpecker.Agent, error)
            ListAgents(context.Context) ([]*woodpecker.Agent, error)
        }

   We'll implement the same.

   But note: we are not given the woodpecker.Agent structure. We assume it has at least an ID and maybe other fields.

   We'll create the Oracle agent with:

        ID: the instance OCID
        // and maybe we set labels? The Provider interface doesn't require labels in the Agent, but the woodpecker.Agent might have them.

   However, the woodpecker.Agent is from "go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker". We don't have its definition.

   We'll assume it has:

        ID string
        // and maybe we can set Labels? But the Provider's ListAgents returns []*woodpecker.Agent.

   We'll set the ID to the instance OCID and leave other fields as zero? Or we can set more if needed.

   Since we are not sure, we'll follow the existing providers. But we don't have the code for the existing providers.

   We'll do as the aws provider does (which we don't see). We must rely on the pattern.

   Alternatively, we can look at the hetznercloud or scaleway providers? But we don't have them.

   Given the constraints, we'll implement a minimal Oracle provider that sets the ID and hope that the woodpecker.Agent only requires the ID for deletion and listing.

   However, note: the DeleteAgent method takes a *woodpecker.Agent. We can use the ID from that agent to delete the instance.

   So in DeleteAgent, we do:

        instanceOCID := agent.ID
        // terminate the instance with that OCID

   And in ListAgents, we return a slice of woodpecker.Agent with the ID set to the instance OCID.

   We'll also need to tag our instances so we can list only ours? Or we can list by compartment and then filter by a tag that we set when creating.

   We'll set a tag when creating the instance: for example, "woodpecker-autoscaler" = "true" and maybe the pool ID.

   Then in ListAgents, we list instances in the compartment that have that tag and are in running state.

   Steps for CreateAgent:

        - Create an instance in the specified compartment, with the specified image (from config.Image), and shape? We don't have a shape flag.

   We need to add a shape flag? Or use a default?

   The config has an Image field. We can use that for the image OCID.

   But we also need a shape. We don't have that in config. We can add a flag for shape? Or use a default.

   Since the issue does not specify, we'll add a flag for shape in the Oracle provider.

   We'll add:

        &cli.StringFlag{
            Name:    "oracle-shape",
            Value:   "VM.Standard.E2.1.Micro", // free tier eligible
            Usage:   "OCI instance shape",
            Sources: cli.EnvVars("WOODPECKER_ORACLE_SHAPE"),
        },

   And then in the provider, we use that shape.

   We'll also need to pass the user data? The config has UserData field. We can pass that as user data (cloud-init) to the instance.

   Steps:

        1. Complete the truncated flag in flags.go.
        2. Add Oracle flags to flags.go.
        3. In main.go, add the oracle case and import the oracle provider.
        4. Create providers/oracle.go with the OracleProvider implementation.

   Let's write the code accordingly.

   Note: We are to write ONLY code, no explanations.

   We'll break it down by file.

   IMPORTANT: We must start each file with "
