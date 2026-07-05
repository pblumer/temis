package bench;

import java.io.File;
import java.math.BigDecimal;
import java.util.Collections;
import java.util.concurrent.TimeUnit;

import org.kie.dmn.api.core.DMNContext;
import org.kie.dmn.api.core.DMNModel;
import org.kie.dmn.api.core.DMNResult;
import org.kie.dmn.api.core.DMNRuntime;
import org.kie.dmn.core.internal.utils.DMNRuntimeBuilder;
import org.kie.internal.io.ResourceFactory;

import org.openjdk.jmh.annotations.Benchmark;
import org.openjdk.jmh.annotations.BenchmarkMode;
import org.openjdk.jmh.annotations.Level;
import org.openjdk.jmh.annotations.Mode;
import org.openjdk.jmh.annotations.OutputTimeUnit;
import org.openjdk.jmh.annotations.Scope;
import org.openjdk.jmh.annotations.Setup;
import org.openjdk.jmh.annotations.State;

/**
 * Drools DMN engine benchmark over the SAME model files Temis is measured
 * against ({@code ../models/*.dmn}). The DMN model is compiled once in
 * {@link #setup()} (mirroring Temis compiling once); each measured op builds a
 * fresh context and calls {@code evaluateAll}, mirroring Temis building an Input
 * map and calling Evaluate.
 *
 * Throughput mode reports ops/s (== decisions/s); AverageTime reports ns/op.
 * Reproduce via ../README.md.
 */
@State(Scope.Benchmark)
@BenchmarkMode({Mode.Throughput, Mode.AverageTime})
@OutputTimeUnit(TimeUnit.SECONDS)
public class DmnBench {

    private DMNRuntime stringRuntime;
    private DMNModel stringModel;

    private DMNRuntime numericRuntime;
    private DMNModel numericModel;

    private static DMNRuntime runtimeFor(File dmn) {
        return DMNRuntimeBuilder.fromDefaults()
                .buildConfiguration()
                .fromResources(Collections.singletonList(ResourceFactory.newFileResource(dmn)))
                .getOrElseThrow(RuntimeException::new);
    }

    @Setup(Level.Trial)
    public void setup() {
        String dir = System.getProperty("models.dir", "../models");

        stringRuntime = runtimeFor(new File(dir, "string-table.dmn"));
        stringModel = stringRuntime.getModels().get(0);

        numericRuntime = runtimeFor(new File(dir, "numeric-table.dmn"));
        numericModel = numericRuntime.getModels().get(0);

        // Parity check: fail fast if Drools does not produce the same outputs as
        // Temis (m8, g5), so the comparison can never silently drift.
        Object menu = evalString();
        if (!"m8".equals(menu)) {
            throw new IllegalStateException("string-table parity: expected m8, got " + menu);
        }
        Object grade = evalNumeric();
        if (!"g5".equals(grade)) {
            throw new IllegalStateException("numeric-table parity: expected g5, got " + grade);
        }
    }

    private Object evalString() {
        DMNContext ctx = stringRuntime.newContext();
        ctx.set("Season", "Winter");
        ctx.set("Region", "R8");
        DMNResult r = stringRuntime.evaluateAll(stringModel, ctx);
        return r.getContext().get("Menu");
    }

    private Object evalNumeric() {
        DMNContext ctx = numericRuntime.newContext();
        ctx.set("A", BigDecimal.valueOf(55));
        ctx.set("B", BigDecimal.ZERO);
        ctx.set("C", BigDecimal.ZERO);
        ctx.set("D", BigDecimal.ZERO);
        DMNResult r = numericRuntime.evaluateAll(numericModel, ctx);
        return r.getContext().get("Grade");
    }

    @Benchmark
    public Object stringTable() {
        return evalString();
    }

    @Benchmark
    public Object numericTable() {
        return evalNumeric();
    }
}
