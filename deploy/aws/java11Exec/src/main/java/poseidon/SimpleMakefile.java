package poseidon;

import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.HashMap;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

// NoMakefileFoundException is thrown if no makefile could be found.
class NoMakefileFoundException extends Exception {}

// NoMakeCommandException is thrown if no make command is called.
class NoMakeCommandException extends Exception {}

// InvalidMakefileException is thrown if there is no valid rule to be executed.
class InvalidMakefileException extends Exception {}

// SimpleMakefile adds limited support for the execution of a makefile as passed command. The default Java image does not contain make.
class SimpleMakefile {

    // This pattern validates if a command is a make command.
    private static final Pattern isMakeCommand = Pattern.compile("^make(?:\\s+(?<startRule>\\w*))?$");

    // This pattern identifies the rules in a makefile.
    private static final Pattern makeRules = Pattern.compile("(?<name>.*):\\n(?<commands>(?:\\t.+\\n?)*)");

    // The make rule that will get executed.
    private String startRule = null;

    // The rules included in the makefile.
    private final Map<String, String[]> rules = new HashMap<>();


    // Unwrapps the passed command. We expect a "sh -c" wrapped command.
    public static String unwrapCommand(String[] cmd) {
        return cmd[cmd.length - 1];
    }

    // Wrapps the passed command with "sh -c".
    public static String[] wrapCommand(String cmd) {
        return new String[]{"sh", "-c", cmd};
    }

    // getMakefile returns the makefile out of the passed files map.
    private static String getMakefile(Map<String, String> files) throws NoMakefileFoundException {
        String makefileB64;
        if (files.containsKey("Makefile")) {
            makefileB64 = files.get("Makefile");
        } else if (files.containsKey("makefile")) {
            makefileB64 = files.get("makefile");
        } else {
            throw new NoMakefileFoundException();
        }

        return new String(Base64.getDecoder().decode(makefileB64), StandardCharsets.UTF_8);
    }

    public SimpleMakefile(String[] cmd, Map<String, String> files) throws NoMakeCommandException, NoMakefileFoundException {
        String makeCommand = unwrapCommand(cmd);
        Matcher makeCommandMatcher = isMakeCommand.matcher(makeCommand);
        if (!makeCommandMatcher.find()) {
            throw new NoMakeCommandException();
        }
        String ruleArgument = makeCommandMatcher.group("startRule");
        if (!ruleArgument.isEmpty()) {
            this.startRule = ruleArgument;
        }

        this.parseRules(getMakefile(files));
    }

    // parseRules uses the passed makefile to parse rules into the objet's map "rules".
    private void parseRules(String makefile) {
        Matcher makeRuleMatcher = makeRules.matcher(makefile);
        while (makeRuleMatcher.find()) {
            String ruleName = makeRuleMatcher.group("name");
            if (startRule == null) {
                startRule = ruleName;
            }

            String[] ruleCommands = makeRuleMatcher.group("commands").split("\n");
            String[] trimmedCommands = new String[ruleCommands.length];
            for (int i = 0; i < ruleCommands.length; i++) {
                trimmedCommands[i] = ruleCommands[i].trim();
            }

            rules.put(ruleName, trimmedCommands);
        }
    }

    // getCommand returns a bash line of commands that would be executed by the makefile.
    public String[] getCommand() throws InvalidMakefileException {
        if ((this.startRule == null) || !rules.containsKey(this.startRule)) {
            throw new InvalidMakefileException();
        }

        String makeCommand = String.join(" && ", rules.get(this.startRule));
        return wrapCommand(makeCommand);
    }
}
