package bench;

import java.io.File;
import java.math.BigDecimal;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
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
 * against ({@code ../models/*.dmn}). Each model's DMN is compiled once in
 * {@link #setup()} (mirroring Temis compiling once); every measured op builds a
 * fresh context and calls {@code evaluateAll}, mirroring Temis building an Input
 * map and calling Evaluate.
 *
 * Throughput mode reports ops/s (== decisions/s); AverageTime reports ns/op.
 * A parity check in setup fails fast unless Drools produces the exact same
 * output as Temis for each scenario. Reproduce via ../README.md.
 */
@State(Scope.Benchmark)
@BenchmarkMode({Mode.Throughput, Mode.AverageTime})
@OutputTimeUnit(TimeUnit.SECONDS)
public class DmnBench {

    // One compiled model per scenario, loaded once.
    private DMNRuntime stringRt, numericRt, arithRt, drgRt, collectRt;
    private DMNModel stringM, numericM, arithM, drgM, collectM;

    private static DMNRuntime runtimeFor(File dmn) {
        return DMNRuntimeBuilder.fromDefaults()
                .buildConfiguration()
                .fromResources(Collections.singletonList(ResourceFactory.newFileResource(dmn)))
                .getOrElseThrow(RuntimeException::new);
    }

    @Setup(Level.Trial)
    public void setup() {
        String dir = System.getProperty("models.dir", "../models");

        stringRt = runtimeFor(new File(dir, "string-table.dmn"));
        stringM = stringRt.getModels().get(0);
        numericRt = runtimeFor(new File(dir, "numeric-table.dmn"));
        numericM = numericRt.getModels().get(0);
        arithRt = runtimeFor(new File(dir, "arithmetic.dmn"));
        arithM = arithRt.getModels().get(0);
        drgRt = runtimeFor(new File(dir, "drg-chain.dmn"));
        drgM = drgRt.getModels().get(0);
        collectRt = runtimeFor(new File(dir, "collect-table.dmn"));
        collectM = collectRt.getModels().get(0);

        // Parity checks: every scenario must match the exact output Temis produces.
        expect("string-table", "m8", evalString());
        expect("numeric-table", "g5", evalNumeric());
        expectNum("arithmetic", new BigDecimal("21.5"), evalArithmetic());
        expectNum("drg-chain", new BigDecimal("10"), evalDrgChain());
        List<?> tags = evalCollect();
        if (!Arrays.asList("low", "mid", "spot").equals(tags)) {
            throw new IllegalStateException("collect-table parity: expected [low, mid, spot], got " + tags);
        }
    }

    private static void expect(String what, Object want, Object got) {
        if (!want.equals(got)) {
            throw new IllegalStateException(what + " parity: expected " + want + ", got " + got);
        }
    }

    private static void expectNum(String what, BigDecimal want, Object got) {
        if (!(got instanceof BigDecimal) || want.compareTo((BigDecimal) got) != 0) {
            throw new IllegalStateException(what + " parity: expected " + want + ", got " + got);
        }
    }

    private Object evalString() {
        DMNContext c = stringRt.newContext();
        c.set("Season", "Winter");
        c.set("Region", "R8");
        return stringRt.evaluateAll(stringM, c).getContext().get("Menu");
    }

    private Object evalNumeric() {
        DMNContext c = numericRt.newContext();
        c.set("A", BigDecimal.valueOf(55));
        c.set("B", BigDecimal.ZERO);
        c.set("C", BigDecimal.ZERO);
        c.set("D", BigDecimal.ZERO);
        return numericRt.evaluateAll(numericM, c).getContext().get("Grade");
    }

    private Object evalArithmetic() {
        DMNContext c = arithRt.newContext();
        c.set("A", BigDecimal.valueOf(6));
        c.set("B", BigDecimal.valueOf(7));
        return arithRt.evaluateAll(arithM, c).getContext().get("R");
    }

    private Object evalDrgChain() {
        DMNContext c = drgRt.newContext();
        c.set("Seed", BigDecimal.ZERO);
        return drgRt.evaluateAll(drgM, c).getContext().get("D10");
    }

    private List<?> evalCollect() {
        DMNContext c = collectRt.newContext();
        c.set("Score", BigDecimal.valueOf(5));
        Object out = collectRt.evaluateAll(collectM, c).getContext().get("Tags");
        return (List<?>) out;
    }

    @Benchmark public Object stringTable()  { return evalString(); }
    @Benchmark public Object numericTable() { return evalNumeric(); }
    @Benchmark public Object arithmetic()   { return evalArithmetic(); }
    @Benchmark public Object drgChain()     { return evalDrgChain(); }
    @Benchmark public Object collectTable() { return evalCollect(); }
}
