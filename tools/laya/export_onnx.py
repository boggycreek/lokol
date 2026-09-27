#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = [
#     "laya",
#     "onnx",
# ]
# ///
"""
Offline ONNX Exporter for Laya ModernBERT Decision Model.

Exports Laya's non-autoregressive DecisionModel to an optimized ONNX CPU graph
for AVX2 execution in Go without Python or PyTorch runtime dependencies.
"""

import argparse
import os
import sys


def export_laya_onnx(output_path: str = "tools/laya/laya_cpu.onnx"):
    import torch
    import laya

    print("Loading Laya model from convaiinnovations/laya...")
    agent = laya.load("convaiinnovations/laya", device="cpu")
    model = agent.model
    model.eval()

    print(f"Exporting model to ONNX at {output_path}...")
    os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)

    # Laya's forward takes:
    # b = {"input_ids": tensor, "attention_mask": tensor, ...}
    # To export cleanly, we wrap the model forward call
    class ONNXWrapper(torch.nn.Module):
        def __init__(self, m):
            super().__init__()
            self.m = m

        def forward(self, input_ids, attention_mask):
            b = {
                "input_ids": input_ids,
                "attention_mask": attention_mask,
            }
            logits, act = self.m(b)
            return logits, act

    wrapper = ONNXWrapper(model)

    dummy_input_ids = torch.randint(0, 50000, (1, 64), dtype=torch.long)
    dummy_attention_mask = torch.ones((1, 64), dtype=torch.long)

    torch.onnx.export(
        wrapper,
        (dummy_input_ids, dummy_attention_mask),
        output_path,
        export_params=True,
        opset_version=18,
        do_constant_folding=True,
        input_names=["input_ids", "attention_mask"],
        output_names=["logits", "act"],
        dynamic_axes={
            "input_ids": {0: "batch_size", 1: "sequence_length"},
            "attention_mask": {0: "batch_size", 1: "sequence_length"},
            "logits": {0: "batch_size"},
            "act": {0: "batch_size"},
        },
    )
    print(f"Export complete: {output_path} ({os.path.getsize(output_path) / (1024 * 1024):.1f} MB)")


def main():
    parser = argparse.ArgumentParser(description="Export Laya Decision Model to ONNX CPU format")
    parser.add_argument("--output", type=str, default="tools/laya/laya_cpu.onnx", help="Path to write ONNX file")
    args = parser.parse_args()

    try:
        export_laya_onnx(args.output)
    except Exception as e:
        print(f"Export error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
