package poseidon;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.Base64;
import java.util.HashMap;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import java.util.stream.Collectors;

// NoMakefileFoundException is thrown if no makefile could be found.
class NoMakefileFoundException extends Exception {}

// NoMakeCommandException is thrown if no make command is called.
class NoMakeCommandException extends Exception {}

// InvalidMakefileException is thrown if there is no valid rule to be executed.
class InvalidMakefileException extends Exception {}

// SimpleMakefile adds limited support for the execution of a makefile as passed command. The default Java image does not contain make.
class SimpleMakefile {

    // This pattern validates if a command is a make command.
    private static final Pattern isMakeCommand = Pattern.compile("^make(?:\\s+(?<startRule>\\w*))?(?<assignments>(?:.*?=.*?)+)?$");

    // This pattern identifies the rules in a makefile.
    private static final Pattern makeRules = Pattern.compile("(?<name>.*):\\r?\\n(?<commands>(?:\\t.+\\r?\\n?)*)");

    // The first rule of the makefile.
    private String firstRule = null;

    // The rules included in the makefile.
    private final Map<String, String[]> rules = new HashMap<>();


    private static String concatCommands(String[] commands) {
        return String.join(" && ", commands);
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

    public SimpleMakefile(Map<String, String> files) throws NoMakefileFoundException {
        this.parseRules(getMakefile(files));
    }

    // parseRules uses the passed makefile to parse rules into the objet's map "rules".
    private void parseRules(String makefile) {
        Matcher makeRuleMatcher = makeRules.matcher(makefile);
        while (makeRuleMatcher.find()) {
            String ruleName = makeRuleMatcher.group("name");
            if (firstRule == null) {
                firstRule = ruleName;
            }

            String[] ruleCommands = makeRuleMatcher.group("commands").split("\n");
            String[] trimmedCommands = Arrays.stream(ruleCommands)
                    .map(String::trim)
                    .map(s -> s.startsWith("@") ? s.substring(1) : s)
                    .toArray(String[]::new);

            rules.put(ruleName, trimmedCommands);
        }
    }

    // getCommand returns a bash line of commands that would be executed by the passed rule.
    public String getCommand(String rule) {
        if (rule == null || rule.isEmpty()) {
            rule = this.firstRule;
        }

        return concatCommands(rules.get(rule));
    }

    // getAssignmentPart returns the key or value of the passed assignment depending on the flag firstPart.
    private String getAssignmentPart(String assignment, boolean firstPart) {
        String[] parts = assignment.split("=");

        if (firstPart) {
            return parts[0];
        } else {
            return parts[1].replaceAll("^\\\"|\\\"$", "");
        }
    }

    // injectAssignments applies all set assignments in the command string.
    private String injectAssignments(String command, String assignments) {
        String result = command;
        Map<String, String> map = Arrays.stream(assignments.split(" "))
                .filter(s -> !s.isEmpty())
                .collect(Collectors.toMap(s -> getAssignmentPart(s, true), s -> getAssignmentPart(s, false)));

        for (Map.Entry<String, String> entry : map.entrySet()) {
            result = result.replaceAll("\\$\\{" + entry.getKey() + "\\}", entry.getValue());
        }

        return result;
    }

    // parseCommand returns a bash line of commands that would be executed by the passed command.
    public String parseCommand(String shellCommand) throws InvalidMakefileException, NoMakeCommandException {
        Matcher makeCommandMatcher = isMakeCommand.matcher(shellCommand);
        if (!makeCommandMatcher.find()) {
            throw new NoMakeCommandException();
        }

        String ruleArgument = makeCommandMatcher.group("startRule");
        if (ruleArgument.isEmpty()) {
            ruleArgument = this.firstRule;
        }

        if ((this.firstRule == null) || !rules.containsKey(ruleArgument)) {
            throw new InvalidMakefileException();
        }

        String command = getCommand(ruleArgument);
        String assignments = makeCommandMatcher.group("assignments");
        return injectAssignments(command, (assignments != null) ? assignments : "");
    }
}
